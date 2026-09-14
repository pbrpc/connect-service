package diagnostics

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectinprocess"
	"google.golang.org/protobuf/proto"

	healthpb "git.sonicoriginal.software/grpc-connect-protos/health"
	"git.sonicoriginal.software/grpc-connect-protos/health/healthconnect"
)

// healthStub answers Check with status, or err when set.
type healthStub struct {
	healthconnect.UnimplementedHealthHandler

	status healthpb.HealthCheckResponse_ServingStatus
	err    error
}

func (h *healthStub) Check(
	context.Context, *healthpb.HealthCheckRequest,
) (*healthpb.HealthCheckResponse, error) {
	if h.err != nil {
		return nil, h.err
	}

	return &healthpb.HealthCheckResponse{Status: h.status}, nil
}

// newServingDependency is a client on a peer that reports itself as serving.
func newServingDependency() *connect.Client {
	return newDependency(&healthStub{status: healthpb.HealthCheckResponse_SERVING})
}

// newFailingDependency is a client on a peer whose health check fails with err.
func newFailingDependency(err error) *connect.Client {
	return newDependency(&healthStub{err: err})
}

// newDependency serves handler in-process and answers with a client on it.
func newDependency(handler healthconnect.HealthHandler) *connect.Client {
	rpc := connect.NewServer()
	healthconnect.RegisterHealthHandler(rpc, handler)

	return connect.NewClient(connectinprocess.New(rpc))
}

// addressStub answers Address with a fixed replica.
type addressStub string

func (a addressStub) Address() string { return string(a) }

// httpClientStub stands in for the HTTP client a target check dials with.
// It records the request and answers every one with a serving health
// response, or fails every one with err.
type httpClientStub struct {
	err     error
	request *http.Request
}

func (s *httpClientStub) Do(request *http.Request) (*http.Response, error) {
	s.request = request

	if s.err != nil {
		return nil, s.err
	}

	body, _ := proto.Marshal(&healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING})

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/proto"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    request,
	}, nil
}

func TestNewDependencyCheck(t *testing.T) {
	t.Run("reports a serving dependency", func(t *testing.T) {
		address := "cache:443"
		check := NewDependencyCheck(newServingDependency(), address)

		got, err := check(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Address != address {
			t.Errorf("address = %q, want %q", got.Address, address)
		}
		if want := healthpb.HealthCheckResponse_SERVING.String(); got.Serving != want {
			t.Errorf("serving = %q, want %q", got.Serving, want)
		}
		if got.State != StateReachable {
			t.Errorf("state = %q, want %q", got.State, StateReachable)
		}
		if got.LastChecked == 0 {
			t.Error("last checked was not recorded")
		}
		if got.Details == nil {
			t.Error("details map is nil")
		}
	})

	t.Run("reports the dependency when the health check fails", func(t *testing.T) {
		address := "queue:443"
		wantErr := connect.NewError(connect.CodeUnavailable, "health service unavailable")
		check := NewDependencyCheck(newFailingDependency(wantErr), address)

		got, err := check(t.Context())
		if connect.CodeOf(err) != connect.CodeUnavailable {
			t.Fatalf("error = %v, want Unavailable", err)
		}
		if got.Address != address {
			t.Errorf("address = %q, want %q", got.Address, address)
		}
		if want := healthpb.HealthCheckResponse_UNKNOWN.String(); got.Serving != want {
			t.Errorf("serving = %q, want %q", got.Serving, want)
		}
		if got.State != StateUnreachable {
			t.Errorf("state = %q, want %q", got.State, StateUnreachable)
		}
	})
}

func TestNewUpstreamCheck(t *testing.T) {
	t.Run("reports the replica the upstream is on", func(t *testing.T) {
		check := NewUpstreamCheck(newServingDependency(), addressStub("10.0.0.1:50054"))

		got, err := check(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Address != "10.0.0.1:50054" {
			t.Errorf("address = %q, want 10.0.0.1:50054", got.Address)
		}
		if want := healthpb.HealthCheckResponse_SERVING.String(); got.Serving != want {
			t.Errorf("serving = %q, want %q", got.Serving, want)
		}
	})

	t.Run("reports no replica when the health check fails", func(t *testing.T) {
		wantErr := connect.NewError(connect.CodeUnavailable, "health service unavailable")
		check := NewUpstreamCheck(newFailingDependency(wantErr), addressStub(""))

		got, err := check(t.Context())
		if connect.CodeOf(err) != connect.CodeUnavailable {
			t.Fatalf("error = %v, want Unavailable", err)
		}
		if got.Address != "" {
			t.Errorf("address = %q, want empty", got.Address)
		}
	})
}

func TestNewTargetCheck(t *testing.T) {
	t.Run("reports a serving target under its address", func(t *testing.T) {
		address := "cache:443"
		httpClient := &httpClientStub{}
		check := NewTargetCheck(httpClient, address)

		got, err := check(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Address != address {
			t.Errorf("address = %q, want %q", got.Address, address)
		}
		if got.State != StateReachable {
			t.Errorf("state = %q, want %q", got.State, StateReachable)
		}
		if want := "http://" + address + healthconnect.HealthCheckProcedure; httpClient.request.URL.String() != want {
			t.Errorf("request URL = %q, want %q", httpClient.request.URL, want)
		}
	})

	t.Run("reports an unreachable target", func(t *testing.T) {
		address := "queue:443"
		check := NewTargetCheck(&httpClientStub{err: errors.New("connection refused")}, address)

		got, err := check(t.Context())
		if err == nil {
			t.Fatal("expected error")
		}
		if got.Address != address {
			t.Errorf("address = %q, want %q", got.Address, address)
		}
		if want := healthpb.HealthCheckResponse_UNKNOWN.String(); got.Serving != want {
			t.Errorf("serving = %q, want %q", got.Serving, want)
		}
		if got.State != StateUnreachable {
			t.Errorf("state = %q, want %q", got.State, StateUnreachable)
		}
	})
}
