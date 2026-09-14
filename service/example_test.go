package service_test

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sonicoriginal.software/logger"

	foundationclient "git.sonicoriginal.software/connect-foundation/client"
	foundationotel "git.sonicoriginal.software/connect-foundation/otel"
	foundation "git.sonicoriginal.software/connect-foundation/server"

	"git.sonicoriginal.software/connect-service/diagnostics"
	"git.sonicoriginal.software/connect-service/health"
	"git.sonicoriginal.software/connect-service/service"
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

	// The process context. The shutdown builds its deadline on this one, which
	// is why the signal cancels a child of it rather than this.
	ctx := context.Background()

	serveCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverName := foundation.Name("example")

	log, flush, err := foundationotel.Init(ctx, serverName, foundation.Version())
	if err != nil {
		slog.Default().Error("Failed to initialize telemetry", slog.Any("error", err))
		return
	}

	ctx = logger.ContextWithLogger(ctx, log)

	// New installs an interceptor that puts this logger into every request
	// context, so it has to be built after the logger is complete.
	srv := foundation.New(log)

	// Deferred before anything else can fail, so every path out of here stops
	// the server and exports what it logged on the way.
	defer foundation.HandleGracefulShutdown(ctx, log, srv.HTTP, flush, cleanupTimeout)

	// Checks for the upstream services this one depends on, keyed by the name
	// diagnostics reports them under. Each is the client the service actually
	// uses, so what is reported is what is in use.
	checks := diagnostics.Checks{}

	if upstreamAddress := os.Getenv("UPSTREAM_ADDRESS"); upstreamAddress != "" {
		upstream := foundationclient.New(
			foundationclient.NewHTTPClient(nil), foundationclient.BaseURL(upstreamAddress), nil,
		)

		checks["upstream"] = diagnostics.NewDependencyCheck(upstream, upstreamAddress)
	}

	healthSrv := health.NewServer()

	// The returned method list names what this server exposes beyond the
	// infrastructure endpoints. A service that advertises itself somewhere
	// hands it on; this one has nowhere to advertise.
	_, err = service.Register(srv.RPC, srv.Mux, healthSrv, checks, registerEcho)
	if err != nil {
		log.Error("Failed to register services", slog.Any("error", err))
		return
	}

	lis, err := foundation.Listen()
	if err != nil {
		log.Error("Failed to create listener", slog.Any("error", err))
		return
	}

	log = log.With(slog.String("address", lis.Addr().String()))

	// Serve mounts what was registered and blocks. A deferred teardown cannot
	// run while it does, so it goes to a goroutine and the select below decides
	// when this returns.
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(lis) }()

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
