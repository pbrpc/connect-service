//revive:disable:package-comments
package health

import (
	"context"
	"sync"

	"connectrpc.com/connect/v2"

	healthpb "git.sonicoriginal.software/grpc-connect-protos/health"
	"git.sonicoriginal.software/grpc-connect-protos/health/healthconnect"
)

// Server serves grpc.health.v1.Health: the serving status of the process, under
// the "" entry, and of each service the caller names. Check and List answer
// with what is recorded; Watch holds a stream and sends each change.
//
// It follows grpc-go's health server: statuses start with the "" entry
// SERVING, an unknown service is NotFound to Check and SERVICE_UNKNOWN to
// Watch, and Shutdown marks everything NOT_SERVING.
type Server struct {
	healthconnect.UnimplementedHealthHandler

	mu       sync.Mutex
	statuses map[string]healthpb.HealthCheckResponse_ServingStatus
	// waiters holds, per service, the channel closed on that service's next
	// change. Created when something first waits on the service, replaced on
	// each change.
	waiters  map[string]chan struct{}
	shutdown bool
}

// NewServer answers with a server whose "" entry is SERVING: the process is up.
func NewServer() *Server {
	return &Server{
		statuses: map[string]healthpb.HealthCheckResponse_ServingStatus{
			"": healthpb.HealthCheckResponse_SERVING,
		},
		waiters: map[string]chan struct{}{},
	}
}

// SetServingStatus records status for service, waking every Watch on it.
// After Shutdown it does nothing, since NOT_SERVING is then the answer for
// everything.
func (s *Server) SetServingStatus(service string, status healthpb.HealthCheckResponse_ServingStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.shutdown {
		return
	}

	s.set(service, status)
}

// Shutdown marks every service NOT_SERVING and holds them there.
func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.shutdown = true

	for service := range s.statuses {
		s.set(service, healthpb.HealthCheckResponse_NOT_SERVING)
	}
}

// set records status and wakes the waiters. Called with the lock held.
func (s *Server) set(service string, status healthpb.HealthCheckResponse_ServingStatus) {
	s.statuses[service] = status

	if waiter, found := s.waiters[service]; found {
		close(waiter)
		delete(s.waiters, service)
	}
}

// status answers with service's recorded status, and whether it has one.
func (s *Server) status(service string) (healthpb.HealthCheckResponse_ServingStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, found := s.statuses[service]

	return status, found
}

// watch answers with service's status as Watch reports it, and the channel
// closed on its next change. Both are read under one lock, so a change landing
// after the read closes the channel handed back and is not slept through.
func (s *Server) watch(service string) (healthpb.HealthCheckResponse_ServingStatus, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, found := s.statuses[service]
	if !found {
		status = healthpb.HealthCheckResponse_SERVICE_UNKNOWN
	}

	waiter, found := s.waiters[service]
	if !found {
		waiter = make(chan struct{})
		s.waiters[service] = waiter
	}

	return status, waiter
}

// Check answers with the status of the named service, or NotFound for one
// that was never recorded.
func (s *Server) Check(
	_ context.Context, request *healthpb.HealthCheckRequest,
) (*healthpb.HealthCheckResponse, error) {
	status, found := s.status(request.GetService())
	if !found {
		return nil, connect.NewError(connect.CodeNotFound, "unknown service")
	}

	return &healthpb.HealthCheckResponse{Status: status}, nil
}

// List answers with the status of every recorded service.
func (s *Server) List(
	_ context.Context, _ *healthpb.HealthListRequest,
) (*healthpb.HealthListResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	statuses := make(map[string]*healthpb.HealthCheckResponse, len(s.statuses))
	for service, status := range s.statuses {
		statuses[service] = &healthpb.HealthCheckResponse{Status: status}
	}

	return &healthpb.HealthListResponse{Statuses: statuses}, nil
}

// Watch sends the named service's status, then sends it again on every change,
// until the stream's context ends. A service not yet recorded is
// SERVICE_UNKNOWN until it is.
func (s *Server) Watch(
	ctx context.Context, request *healthpb.HealthCheckRequest, stream healthconnect.HealthWatchServerStream,
) error {
	return s.watchLoop(ctx, request.GetService(), stream.Send)
}

// watchLoop is Watch over a send function, so the stream Watch is handed can be
// stood in for by a test.
func (s *Server) watchLoop(
	ctx context.Context, service string, send func(*healthpb.HealthCheckResponse) error,
) error {
	for {
		status, changed := s.watch(service)

		if err := send(&healthpb.HealthCheckResponse{Status: status}); err != nil {
			return err
		}

		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
