package service_test

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sonicoriginal.software/logger"

	"github.com/caarlos0/env/v11"
	connectserver "github.com/pbrpc/connect-server"
	httpclient "github.com/pbrpc/http-client"
	"github.com/pbrpc/lifecycle"
	"github.com/pbrpc/otel"
	svc "github.com/pbrpc/service"

	"github.com/pbrpc/connect-service/diagnostics"
	"github.com/pbrpc/connect-service/health"
	"github.com/pbrpc/connect-service/service"
)

const cleanupTimeout = 5 * time.Second

// Example shows how a service wires itself up. The listener and the server are
// the caller's, as are the goroutines; this package only assembles. It has no
// Output comment, so it is compiled but never run — its job is to keep this
// sequence type-checked.
func Example() {
	// Registered first so it runs last, after the teardown below has flushed.
	// Returning rather than calling os.Exit directly is what lets the defers run
	// at all.
	exitCode := 1
	defer func() { os.Exit(exitCode) }()

	configured, err := env.ParseAs[svc.Configuration]()
	if err != nil {
		return
	}
	// The process context. The shutdown builds its deadline on this one, which
	// is why the signal cancels a child of it rather than this.
	ctx := context.Background()

	serveCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverName := configured.Name

	log, flush, err := otel.Init(ctx, serverName, configured.Version)
	if err != nil {
		slog.Default().Error("Failed to initialize telemetry", slog.Any("error", err))
		return
	}

	ctx = logger.ContextWithLogger(ctx, log)

	// New installs an interceptor that puts this logger into every request
	// context, so it has to be built after the logger is complete.
	host, err := connectserver.FromEnv(log)
	if err != nil {
		return
	}

	stack := lifecycle.Stack{}
	stack.Push(flush)
	stack.Push(host.HTTPHost.Server.Shutdown)

	// Deferred before anything else can fail, so every path out of here stops
	// the server and exports what it logged on the way.
	defer lifecycle.HandleGracefulShutdown(ctx, log, &stack, cleanupTimeout)

	// Checks for the upstream services this one depends on, keyed by the name
	// diagnostics reports them under. Each probes over the HTTP client the
	// service actually reaches that upstream with, so what is reported is what
	// is in use.
	checks := diagnostics.Checks{}

	if upstreamAddress := os.Getenv("UPSTREAM_ADDRESS"); upstreamAddress != "" {
		httpClient, err := httpclient.FromEnv(nil)
		if err != nil {
			return
		}

		// The Connect client for the upstream's own procedures is built on the
		// same HTTP client:
		//   foundationclient.New(httpClient, foundationclient.BaseURL(upstreamAddress), nil)
		checks["upstream"] = diagnostics.NewDependencyCheck(httpClient, upstreamAddress)
	}

	healthSrv := health.NewServer()

	// The returned method list names what this server exposes beyond the
	// infrastructure endpoints. A service that advertises itself somewhere
	// hands it on; this one has nowhere to advertise.
	_, err = service.Register(host.Server, host.HTTPHost.Mux, healthSrv, checks, registerEcho)
	if err != nil {
		log.Error("Failed to register services", slog.Any("error", err))
		return
	}

	lis, err := net.Listen("tcp", configured.Address)
	if err != nil {
		log.Error("Failed to create listener", slog.Any("error", err))
		return
	}

	log = log.With(slog.String("address", lis.Addr().String()))

	// Serve mounts what was registered and blocks. A deferred teardown cannot
	// run while it does, so it goes to a goroutine and the select below decides
	// when this returns.
	serveErr := make(chan error, 1)
	go func() { serveErr <- host.Serve(lis) }()

	log.Info("Connect server listening")

	select {
	case err := <-serveErr:
		if err != nil {
			log.Error("Failed to serve", slog.Any("error", err))
		}
	case <-serveCtx.Done():
		exitCode = 0
	}
}
