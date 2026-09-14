package service

import (
	"context"
	"net/http"
	"slices"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pbrpc/connect-service/health"
)

const exampleService = "example.ExampleService"

// echo answers a unary procedure by sending the request back.
func echo(_ context.Context, _ connect.Spec, stream connect.ServerStream) error {
	var request wrapperspb.StringValue
	if err := stream.Receive(&request); err != nil {
		return err
	}

	return stream.Send(&request)
}

// registerExampleService stands in for a caller's registerFn. It registers two
// methods so that deriving service names from them has a duplicate to collapse.
func registerExampleService(rpc *connect.Server) {
	for _, method := range []string{"Create", "Delete"} {
		rpc.Register(connect.Method{
			Spec: connect.Spec{
				StreamType: connect.StreamTypeUnary,
				Procedure:  "/" + exampleService + "/" + method,
			},
			Handler: echo,
		})
	}
}

// registered reports whether a procedure of service was registered on rpc.
func registered(rpc *connect.Server, service string) bool {
	for spec := range rpc.Specs() {
		if serviceName(spec.Procedure) == service {
			return true
		}
	}

	return false
}

// muxStub records the patterns handed to it.
type muxStub struct {
	patterns []string
}

func (m *muxStub) Handle(pattern string, _ http.Handler) {
	m.patterns = append(m.patterns, pattern)
}

func (m *muxStub) handled(pattern string) bool {
	return slices.Contains(m.patterns, pattern)
}

// healthStub records the serving statuses Register set.
type healthStub struct {
	statuses map[string]health.Status
}

func newHealthStub() *healthStub {
	return &healthStub{statuses: map[string]health.Status{}}
}

func (h *healthStub) SetServingStatus(service string, status health.Status) {
	h.statuses[service] = status
}

func (h *healthStub) ServeHTTP(http.ResponseWriter, *http.Request) {}
