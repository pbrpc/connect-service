package diagnostics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pbrpc/connect-service/health"
)

// serverClient stands in for the HTTP client: it hands every request to srv's
// probe handler in-process, so a check runs against the real route with
// nothing listening.
type serverClient struct {
	srv *health.Server
}

func (c *serverClient) Do(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	c.srv.ServeHTTP(recorder, request)

	return recorder.Result(), nil
}

// failingClient fails every request with err.
type failingClient struct {
	err error
}

func (c *failingClient) Do(*http.Request) (*http.Response, error) {
	return nil, c.err
}

// addressStub answers Address with a fixed replica.
type addressStub string

func (a addressStub) Address() string { return string(a) }

func TestNewDependencyCheck(t *testing.T) {
	t.Run("reports a serving dependency", func(t *testing.T) {
		address := "cache:443"
		check := NewDependencyCheck(&serverClient{srv: health.NewServer()}, address)

		got, err := check(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Address != address {
			t.Errorf("address = %q, want %q", got.Address, address)
		}
		if want := string(health.StatusServing); got.Serving != want {
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

	t.Run("reports a dependency that is not serving", func(t *testing.T) {
		srv := health.NewServer()
		srv.Shutdown()

		got, err := NewDependencyCheck(&serverClient{srv: srv}, "cache:443")(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := string(health.StatusNotServing); got.Serving != want {
			t.Errorf("serving = %q, want %q", got.Serving, want)
		}
		if got.State != StateReachable {
			t.Errorf("state = %q, want %q", got.State, StateReachable)
		}
	})

	t.Run("reports the dependency when it cannot be reached", func(t *testing.T) {
		address := "queue:443"
		wantErr := errors.New("connection refused")
		check := NewDependencyCheck(&failingClient{err: wantErr}, address)

		got, err := check(t.Context())
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if got.Address != address {
			t.Errorf("address = %q, want %q", got.Address, address)
		}
		if want := string(health.StatusUnknown); got.Serving != want {
			t.Errorf("serving = %q, want %q", got.Serving, want)
		}
		if got.State != StateUnreachable {
			t.Errorf("state = %q, want %q", got.State, StateUnreachable)
		}
	})
}

func TestNewUpstreamCheck(t *testing.T) {
	t.Run("probes and reports the replica the upstream is on", func(t *testing.T) {
		check := NewUpstreamCheck(&serverClient{srv: health.NewServer()}, addressStub("10.0.0.1:50054"))

		got, err := check(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Address != "10.0.0.1:50054" {
			t.Errorf("address = %q, want 10.0.0.1:50054", got.Address)
		}
		if want := string(health.StatusServing); got.Serving != want {
			t.Errorf("serving = %q, want %q", got.Serving, want)
		}
	})

	t.Run("reports no replica when none is held", func(t *testing.T) {
		check := NewUpstreamCheck(&failingClient{err: errors.New("no replica")}, addressStub(""))

		got, err := check(t.Context())
		if err == nil {
			t.Fatal("expected error")
		}
		if got.Address != "" {
			t.Errorf("address = %q, want empty", got.Address)
		}
		if got.State != StateUnreachable {
			t.Errorf("state = %q, want %q", got.State, StateUnreachable)
		}
	})
}
