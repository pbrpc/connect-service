package health

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serverClient stands in for the HTTP client: it hands every request to srv's
// handler in-process, so Check is exercised against the real probe route with
// nothing listening.
type serverClient struct {
	srv     *Server
	request *http.Request
}

func (c *serverClient) Do(request *http.Request) (*http.Response, error) {
	c.request = request

	recorder := httptest.NewRecorder()
	c.srv.ServeHTTP(recorder, request)

	return recorder.Result(), nil
}

// cannedClient answers every request with the given status and body, or
// fails with err.
type cannedClient struct {
	code int
	body string
	err  error
}

func (c *cannedClient) Do(request *http.Request) (*http.Response, error) {
	if c.err != nil {
		return nil, c.err
	}

	return &http.Response{
		StatusCode: c.code,
		Status:     http.StatusText(c.code),
		Body:       io.NopCloser(strings.NewReader(c.body)),
		Request:    request,
	}, nil
}

func TestCheck(t *testing.T) {
	t.Run("asks the probe route and reports serving", func(t *testing.T) {
		client := &serverClient{srv: NewServer()}

		got, err := Check(t.Context(), client, "cache:443")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != StatusServing {
			t.Errorf("status = %v, want SERVING", got)
		}
		if want := "http://cache:443" + HTTPPath; client.request.URL.String() != want {
			t.Errorf("request URL = %q, want %q", client.request.URL, want)
		}
	})

	t.Run("reports not serving from a 503", func(t *testing.T) {
		srv := NewServer()
		srv.Shutdown()

		got, err := Check(t.Context(), &serverClient{srv: srv}, "cache:443")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != StatusNotServing {
			t.Errorf("status = %v, want NOT_SERVING", got)
		}
	})

	t.Run("reports unknown when the peer cannot be reached", func(t *testing.T) {
		want := errors.New("connection refused")

		got, err := Check(t.Context(), &cannedClient{err: want}, "cache:443")
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want %v", err, want)
		}
		if got != StatusUnknown {
			t.Errorf("status = %v, want UNKNOWN", got)
		}
	})

	t.Run("reports unknown when the answer is not a status", func(t *testing.T) {
		got, err := Check(t.Context(), &cannedClient{code: http.StatusBadGateway, body: "upstream down"}, "cache:443")
		if err == nil {
			t.Fatal("expected error")
		}
		if got != StatusUnknown {
			t.Errorf("status = %v, want UNKNOWN", got)
		}
	})

	t.Run("reports unknown when the answer does not decode", func(t *testing.T) {
		got, err := Check(t.Context(), &cannedClient{code: http.StatusOK, body: "not json"}, "cache:443")
		if err == nil {
			t.Fatal("expected error")
		}
		if got != StatusUnknown {
			t.Errorf("status = %v, want UNKNOWN", got)
		}
	})

	t.Run("reports unknown when the address makes no URL", func(t *testing.T) {
		// A control character fails URL parsing, which is the one way the
		// request itself cannot be built.
		got, err := Check(t.Context(), &cannedClient{}, "cache:443\n")
		if err == nil {
			t.Fatal("expected error")
		}
		if got != StatusUnknown {
			t.Errorf("status = %v, want UNKNOWN", got)
		}
	})
}
