//revive:disable:package-comments
package diagnostics

import (
	"context"
	"time"

	"connectrpc.com/connect/v2/connecthttp"

	diagpb "github.com/pbrpc/connect-protos/diagnostics"

	"github.com/pbrpc/connect-service/health"
)

// Check inspects a dependency
type Check func(context.Context) (*diagpb.ServiceDependency, error)

// Checks is a bag of Checks
type Checks map[string]Check

// The states a dependency is reported in. An HTTP client holds no connection
// state of its own, so what is reported is whether the health probe got
// through.
const (
	StateReachable   = "REACHABLE"
	StateUnreachable = "UNREACHABLE"
)

// healthCheck probes the dependency at address over httpClient and reports it.
func healthCheck(
	ctx context.Context, httpClient connecthttp.HTTPClient, address string,
) (*diagpb.ServiceDependency, error) {
	serving, err := health.Check(ctx, httpClient, address)

	state := StateReachable
	if err != nil {
		state = StateUnreachable
	}

	return &diagpb.ServiceDependency{
		Address:     address,
		Serving:     string(serving),
		State:       state,
		LastChecked: time.Now().Unix(),
		Details:     map[string]string{},
	}, err
}

// Addressed is what a check needs of something that knows which replica a
// dependency is on right now.
type Addressed interface {
	Address() string
}

// addressed reports a fixed address.
type addressed string

func (a addressed) Address() string { return string(a) }

// NewDependencyCheck wires up probing the dependency at address over
// httpClient, typically the client the caller already reaches it with, so
// what is reported is what is in use.
func NewDependencyCheck(httpClient connecthttp.HTTPClient, address string) Check {
	return NewUpstreamCheck(httpClient, addressed(address))
}

// NewUpstreamCheck wires up probing a dependency whose replica is chosen per
// request. upstream supplies the address the dependency is on right now,
// which is what is probed and reported.
func NewUpstreamCheck(httpClient connecthttp.HTTPClient, upstream Addressed) Check {
	return func(ctx context.Context) (*diagpb.ServiceDependency, error) {
		return healthCheck(ctx, httpClient, upstream.Address())
	}
}
