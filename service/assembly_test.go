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

	foundation "git.sonicoriginal.software/connect-foundation/server"
	diagpb "git.sonicoriginal.software/grpc-connect-protos/diagnostics"
	"git.sonicoriginal.software/grpc-connect-protos/diagnostics/diagnosticsconnect"
	healthpb "git.sonicoriginal.software/grpc-connect-protos/health"
	"git.sonicoriginal.software/grpc-connect-protos/health/healthconnect"
	infopb "git.sonicoriginal.software/grpc-connect-protos/info"
	"git.sonicoriginal.software/grpc-connect-protos/info/infoconnect"

	"git.sonicoriginal.software/connect-service/diagnostics"
	"git.sonicoriginal.software/connect-service/health"
	"git.sonicoriginal.software/connect-service/service"
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

// upstreamCheck stands in for a dependency check the caller wrote.
func upstreamCheck(context.Context) (*diagpb.ServiceDependency, error) {
	return &diagpb.ServiceDependency{Address: "upstream:50051", State: diagnostics.StateReachable}, nil
}

// assemble is the whole startup sequence short of listening: the foundation
// server, the caller's service, and Register. It answers with the server and
// the health handle the caller keeps.
func assemble(t *testing.T) (*foundation.Server, *health.Server) {
	t.Helper()

	srv := foundation.New(slog.New(slog.DiscardHandler))
	healthSrv := health.NewServer()
	checks := diagnostics.Checks{dependencyName: upstreamCheck}

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

	t.Run("health reports the process and the caller's service", func(t *testing.T) {
		healthClient := healthconnect.NewHealthClient(client)

		for _, name := range []string{"", exampleService} {
			response, err := healthClient.Check(t.Context(), &healthpb.HealthCheckRequest{Service: name})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if response.GetStatus() != healthpb.HealthCheckResponse_SERVING {
				t.Errorf("%q status = %v, want SERVING", name, response.GetStatus())
			}
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

	t.Run("diagnostics reports the caller's dependency", func(t *testing.T) {
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
	})

	t.Run("the health route answers plain HTTP on the mux", func(t *testing.T) {
		for _, target := range []string{health.HTTPPath, health.HTTPPath + "?service=" + exampleService} {
			recorder := httptest.NewRecorder()
			srv.Mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

			if recorder.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want 200", target, recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), `"SERVING"`) {
				t.Errorf("%s: body = %q, want the status", target, recorder.Body.String())
			}
		}
	})

	t.Run("the caller's status change reaches both routes", func(t *testing.T) {
		healthSrv.SetServingStatus(exampleService, healthpb.HealthCheckResponse_NOT_SERVING)

		response, err := healthconnect.NewHealthClient(client).
			Check(t.Context(), &healthpb.HealthCheckRequest{Service: exampleService})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.GetStatus() != healthpb.HealthCheckResponse_NOT_SERVING {
			t.Errorf("status = %v, want NOT_SERVING", response.GetStatus())
		}

		recorder := httptest.NewRecorder()
		srv.Mux.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, health.HTTPPath+"?service="+exampleService, nil,
		))

		if recorder.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", recorder.Code)
		}
	})
}
