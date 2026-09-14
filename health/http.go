package health

import (
	"encoding/json"
	"net/http"
)

// HTTPPath is where the server answers plain HTTP probes, the ones that can
// only GET: GET /healthz for the process, GET /healthz?service=<name> for one
// service.
const HTTPPath = "/healthz"

// serviceQuery names the query parameter carrying the service.
const serviceQuery = "service"

// Response is the body of a probe answer.
type Response struct {
	Status Status `json:"status"`
}

// ServeHTTP answers a GET with the recorded status: 200 for SERVING, 503 for
// anything else, 404 for a service never recorded. The body is a Response as
// JSON. Any other method is 405.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	status, found := s.Status(r.URL.Query().Get(serviceQuery))
	if !found {
		http.Error(w, "unknown service", http.StatusNotFound)

		return
	}

	code := http.StatusServiceUnavailable
	if status == StatusServing {
		code = http.StatusOK
	}

	// Marshaling a struct of one string cannot fail, so the error is not
	// consulted.
	body, _ := json.Marshal(Response{Status: status})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}
