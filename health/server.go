//revive:disable:package-comments
package health

import "sync"

// Status is a service's serving status, named as grpc.health.v1 names them so
// the answers here and the standard health service's agree.
type Status string

// The statuses a service is reported in.
const (
	StatusUnknown    Status = "UNKNOWN"
	StatusServing    Status = "SERVING"
	StatusNotServing Status = "NOT_SERVING"
)

// Server holds the serving status of the process, under the "" entry, and of
// each service the caller names. It answers plain HTTP probes on HTTPPath.
//
// It follows grpc-go's health server: the "" entry starts SERVING, a service
// never recorded is unknown, and Shutdown marks everything NOT_SERVING.
type Server struct {
	// mu guards statuses: the application writes it from its own goroutines
	// and every probe request reads it from the HTTP server's.
	mu       sync.Mutex
	statuses map[string]Status
	shutdown bool
}

// NewServer answers with a server whose "" entry is SERVING: the process is up.
func NewServer() *Server {
	return &Server{
		statuses: map[string]Status{"": StatusServing},
	}
}

// SetServingStatus records status for service. After Shutdown it does
// nothing, since NOT_SERVING is then the answer for everything.
func (s *Server) SetServingStatus(service string, status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.shutdown {
		return
	}

	s.statuses[service] = status
}

// Shutdown marks every service NOT_SERVING and holds them there.
func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.shutdown = true

	for service := range s.statuses {
		s.statuses[service] = StatusNotServing
	}
}

// Status answers with service's recorded status, and whether it has one.
func (s *Server) Status(service string) (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, found := s.statuses[service]

	return status, found
}
