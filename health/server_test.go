package health

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectinprocess"

	healthpb "git.sonicoriginal.software/grpc-connect-protos/health"
	"git.sonicoriginal.software/grpc-connect-protos/health/healthconnect"
)

const exampleService = "example.ExampleService"

// newClient serves srv in-process and answers with a client on it, so every
// RPC in these tests goes through the generated handler and nothing listens.
func newClient(srv *Server) healthconnect.HealthClient {
	rpc := connect.NewServer()
	healthconnect.RegisterHealthHandler(rpc, srv)

	return healthconnect.NewHealthClient(connect.NewClient(connectinprocess.New(rpc)))
}

// check fails the test unless client answers status for service.
func check(
	t *testing.T, client healthconnect.HealthClient, service string, want healthpb.HealthCheckResponse_ServingStatus,
) {
	t.Helper()

	response, err := client.Check(t.Context(), &healthpb.HealthCheckRequest{Service: service})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.GetStatus() != want {
		t.Errorf("%q status = %v, want %v", service, response.GetStatus(), want)
	}
}

// receive fails the test unless the next message on stream carries want.
func receive(
	t *testing.T, stream healthconnect.HealthWatchClientStream, want healthpb.HealthCheckResponse_ServingStatus,
) {
	t.Helper()

	response, err := stream.Receive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.GetStatus() != want {
		t.Errorf("status = %v, want %v", response.GetStatus(), want)
	}
}

func TestCheck(t *testing.T) {
	t.Run("reports the process serving from the start", func(t *testing.T) {
		check(t, newClient(NewServer()), "", healthpb.HealthCheckResponse_SERVING)
	})

	t.Run("reports a service as recorded", func(t *testing.T) {
		srv := NewServer()
		srv.SetServingStatus(exampleService, healthpb.HealthCheckResponse_NOT_SERVING)

		check(t, newClient(srv), exampleService, healthpb.HealthCheckResponse_NOT_SERVING)
	})

	t.Run("answers NotFound for a service never recorded", func(t *testing.T) {
		_, err := newClient(NewServer()).Check(t.Context(), &healthpb.HealthCheckRequest{Service: exampleService})

		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Fatalf("error = %v, want NotFound", err)
		}
	})
}

func TestList(t *testing.T) {
	srv := NewServer()
	srv.SetServingStatus(exampleService, healthpb.HealthCheckResponse_SERVING)

	response, err := newClient(srv).List(t.Context(), &healthpb.HealthListRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses := response.GetStatuses()
	if len(statuses) != 2 {
		t.Fatalf("statuses = %v, want the process and the service", statuses)
	}
	for _, service := range []string{"", exampleService} {
		if statuses[service].GetStatus() != healthpb.HealthCheckResponse_SERVING {
			t.Errorf("%q status = %v, want SERVING", service, statuses[service].GetStatus())
		}
	}
}

func TestShutdown(t *testing.T) {
	srv := NewServer()
	srv.SetServingStatus(exampleService, healthpb.HealthCheckResponse_SERVING)

	srv.Shutdown()

	client := newClient(srv)
	check(t, client, "", healthpb.HealthCheckResponse_NOT_SERVING)
	check(t, client, exampleService, healthpb.HealthCheckResponse_NOT_SERVING)

	// Nothing comes back up after a shutdown.
	srv.SetServingStatus(exampleService, healthpb.HealthCheckResponse_SERVING)
	check(t, client, exampleService, healthpb.HealthCheckResponse_NOT_SERVING)
}

func TestWatch(t *testing.T) {
	t.Run("sends the status and then each change", func(t *testing.T) {
		srv := NewServer()
		client := newClient(srv)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		stream, err := client.Watch(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		receive(t, stream, healthpb.HealthCheckResponse_SERVING)

		srv.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		receive(t, stream, healthpb.HealthCheckResponse_NOT_SERVING)

		srv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		receive(t, stream, healthpb.HealthCheckResponse_SERVING)

		// Ending the context is what ends the watch.
		cancel()

		if _, err := stream.Receive(); err == nil {
			t.Fatal("expected the stream to end with the context")
		}
	})

	t.Run("reports a service unknown until it is recorded", func(t *testing.T) {
		srv := NewServer()
		client := newClient(srv)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		stream, err := client.Watch(ctx, &healthpb.HealthCheckRequest{Service: exampleService})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		receive(t, stream, healthpb.HealthCheckResponse_SERVICE_UNKNOWN)

		srv.SetServingStatus(exampleService, healthpb.HealthCheckResponse_SERVING)
		receive(t, stream, healthpb.HealthCheckResponse_SERVING)
	})

	t.Run("wakes every watcher of a service", func(t *testing.T) {
		srv := NewServer()
		client := newClient(srv)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		first, err := client.Watch(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		second, err := client.Watch(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		receive(t, first, healthpb.HealthCheckResponse_SERVING)
		receive(t, second, healthpb.HealthCheckResponse_SERVING)

		srv.Shutdown()

		receive(t, first, healthpb.HealthCheckResponse_NOT_SERVING)
		receive(t, second, healthpb.HealthCheckResponse_NOT_SERVING)
	})

	t.Run("ends when the stream cannot be sent on", func(t *testing.T) {
		want := errors.New("stream closed")

		err := NewServer().watchLoop(t.Context(), "", func(*healthpb.HealthCheckResponse) error {
			return want
		})

		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want %v", err, want)
		}
	})
}
