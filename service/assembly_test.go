package service_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectinprocess"
	"google.golang.org/protobuf/types/known/wrapperspb"

	foundation "github.com/pbrpc/connect-foundation/server"
	diagpb "github.com/pbrpc/connect-protos/diagnostics"
	"github.com/pbrpc/connect-protos/diagnostics/diagnosticsconnect"
	infopb "github.com/pbrpc/connect-protos/info"
	"github.com/pbrpc/connect-protos/info/infoconnect"

	"github.com/pbrpc/connect-service/diagnostics"
	"github.com/pbrpc/connect-service/health"
	"github.com/pbrpc/connect-service/service"
)

const (
	exampleService = "example.ExampleService"
	echoProcedure  = "/" + exampleService + "/Echo"
	dependencyName = "upstream"
)

var echoSpec = connect.Spec{StreamType: connect.StreamTypeUnary, Procedure: echoProcedure}

// registerEcho stands in for the caller's generated registration.
func registerEcho(rpc *connect.Server) {
	rpc.Register(connect.Method{
		Spec: echoSpec,
		Handler: func(_ context.Context, _ connect.Spec, stream connect.ServerStream) error {
			var request wrapperspb.StringValue
			if err := stream.Receive(&request); err != nil {
				return err
			}

			return stream.Send(&request)
		},
	})
}

// upstreamClient stands in for the HTTP client a dependency is reached with:
// it hands every request to a health server's probe route in-process, so the
// dependency check runs against the real route with nothing listening.
type upstreamClient struct {
	srv *health.Server
}

func (c *upstreamClient) Do(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	c.srv.ServeHTTP(recorder, request)

	return recorder.Result(), nil
}

// assemble is the whole startup sequence short of listening: the foundation
// server, the caller's service, and Register. It answers with the server and
// the health handle the caller keeps.
func assemble(t *testing.T) (*foundation.Server, *health.Server) {
	t.Helper()

	srv := foundation.New(slog.New(slog.DiscardHandler))
	healthSrv := health.NewServer()
	checks := diagnostics.Checks{
		dependencyName: diagnostics.NewDependencyCheck(&upstreamClient{srv: health.NewServer()}, "upstream:50051"),
	}

	methods, err := service.Register(srv.RPC, srv.Mux, healthSrv, checks, registerEcho)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(methods) != 1 || methods[0] != echoProcedure {
		t.Fatalf("methods = %v, want the echo procedure alone", methods)
	}

	// What Serve would do before listening.
	srv.Mount()

	return srv, healthSrv
}

// probe sends GET target through the mux and answers with the recording.
func probe(srv *foundation.Server, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	srv.Mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

	return recorder
}

func TestAssembly(t *testing.T) {
	t.Setenv(foundation.EnvServerVersion, "1.2.3")

	srv, healthSrv := assemble(t)

	// Every RPC below goes through the assembled dispatcher and its
	// interceptors, over the in-process transport: no listener, no socket.
	client := connect.NewClient(connectinprocess.New(srv.RPC))

	t.Run("the caller's service answers", func(t *testing.T) {
		var response wrapperspb.StringValue

		if err := client.CallUnary(t.Context(), echoSpec, wrapperspb.String("hello"), &response); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.GetValue() != "hello" {
			t.Errorf("response = %q, want hello", response.GetValue())
		}
	})

	t.Run("info reports the configured version", func(t *testing.T) {
		response, err := infoconnect.NewInfoServiceClient(client).Version(t.Context(), &infopb.VersionRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.GetVersion() != "1.2.3" {
			t.Errorf("version = %q, want 1.2.3", response.GetVersion())
		}
	})

	t.Run("diagnostics probes the caller's dependency", func(t *testing.T) {
		response, err := diagnosticsconnect.NewDiagnosticsServiceClient(client).
			GetDiagnostics(t.Context(), &diagpb.GetDiagnosticsRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		dependency := response.GetServices()[dependencyName]
		if dependency == nil {
			t.Fatalf("services = %v, want %q", response.GetServices(), dependencyName)
		}
		if dependency.GetState() != diagnostics.StateReachable {
			t.Errorf("state = %q, want %q", dependency.GetState(), diagnostics.StateReachable)
		}
		if dependency.GetServing() != string(health.StatusServing) {
			t.Errorf("serving = %q, want SERVING", dependency.GetServing())
		}
	})

	t.Run("the probe route reports the process and the caller's service", func(t *testing.T) {
		for _, target := range []string{health.HTTPPath, health.HTTPPath + "?service=" + exampleService} {
			recorder := probe(srv, target)

			if recorder.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want 200", target, recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), `"SERVING"`) {
				t.Errorf("%s: body = %q, want the status", target, recorder.Body.String())
			}
		}
	})

	t.Run("the caller's status change reaches the probe route", func(t *testing.T) {
		healthSrv.SetServingStatus(exampleService, health.StatusNotServing)

		if recorder := probe(srv, health.HTTPPath+"?service="+exampleService); recorder.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", recorder.Code)
		}
	})
}
