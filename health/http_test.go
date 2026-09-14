package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// get sends a GET for target to srv and answers with the recording.
func get(srv *Server, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

	return recorder
}

func TestServeHTTP(t *testing.T) {
	t.Run("answers 200 for a serving process", func(t *testing.T) {
		recorder := get(NewServer(), HTTPPath)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", recorder.Code)
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q, want application/json", got)
		}
		if got := strings.TrimSpace(recorder.Body.String()); got != `{"status":"SERVING"}` {
			t.Errorf("body = %q, want the serving status", got)
		}
	})

	t.Run("answers 503 for a service not serving", func(t *testing.T) {
		srv := NewServer()
		srv.SetServingStatus(exampleService, StatusNotServing)

		recorder := get(srv, HTTPPath+"?service="+exampleService)

		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), `"NOT_SERVING"`) {
			t.Errorf("body = %q, want the status", recorder.Body.String())
		}
	})

	t.Run("answers 200 for a serving service", func(t *testing.T) {
		srv := NewServer()
		srv.SetServingStatus(exampleService, StatusServing)

		if recorder := get(srv, HTTPPath+"?service="+exampleService); recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", recorder.Code)
		}
	})

	t.Run("answers 404 for a service never recorded", func(t *testing.T) {
		if recorder := get(NewServer(), HTTPPath+"?service="+exampleService); recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", recorder.Code)
		}
	})

	t.Run("answers 405 to anything but GET", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		NewServer().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, HTTPPath, nil))

		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", recorder.Code)
		}
		if got := recorder.Header().Get("Allow"); got != http.MethodGet {
			t.Errorf("allow = %q, want GET", got)
		}
	})
}
