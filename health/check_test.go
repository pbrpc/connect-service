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

// handlerStub answers Check with status, or err when set.
type handlerStub struct {
	healthconnect.UnimplementedHealthHandler

	status healthpb.HealthCheckResponse_ServingStatus
	err    error
}

func (h *handlerStub) Check(
	context.Context, *healthpb.HealthCheckRequest,
) (*healthpb.HealthCheckResponse, error) {
	if h.err != nil {
		return nil, h.err
	}

	return &healthpb.HealthCheckResponse{Status: h.status}, nil
}

// newPeer serves handler in-process and answers with a client on it.
func newPeer(handler healthconnect.HealthHandler) *connect.Client {
	rpc := connect.NewServer()
	healthconnect.RegisterHealthHandler(rpc, handler)

	return connect.NewClient(connectinprocess.New(rpc))
}

func TestCheckClient(t *testing.T) {
	t.Run("serving", func(t *testing.T) {
		client := newPeer(&handlerStub{status: healthpb.HealthCheckResponse_SERVING})

		got, err := Check(t.Context(), client)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != healthpb.HealthCheckResponse_SERVING {
			t.Errorf("status = %v, want SERVING", got)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		client := newPeer(&handlerStub{err: connect.NewError(connect.CodeUnavailable, "health service unavailable")})

		got, err := Check(t.Context(), client)
		if connect.CodeOf(err) != connect.CodeUnavailable {
			t.Fatalf("error = %v, want Unavailable", err)
		}
		if got != healthpb.HealthCheckResponse_UNKNOWN {
			t.Errorf("status = %v, want UNKNOWN", got)
		}
	})

	t.Run("context ended", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		client := newPeer(&handlerStub{status: healthpb.HealthCheckResponse_SERVING})

		got, err := Check(ctx, client)
		if !errors.Is(err, context.Canceled) && connect.CodeOf(err) != connect.CodeCanceled {
			t.Fatalf("error = %v, want the cancellation", err)
		}
		if got != healthpb.HealthCheckResponse_UNKNOWN {
			t.Errorf("status = %v, want UNKNOWN", got)
		}
	})
}
