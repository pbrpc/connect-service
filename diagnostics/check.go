//revive:disable:package-comments
package diagnostics

import (
	"context"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	foundationclient "git.sonicoriginal.software/connect-foundation/client"
	diagpb "git.sonicoriginal.software/grpc-connect-protos/diagnostics"

	"git.sonicoriginal.software/connect-service/health"
)

// Check inspects a dependency
type Check func(context.Context) (*diagpb.ServiceDependency, error)

// Checks is a bag of Checks
type Checks map[string]Check

// The states a dependency is reported in. An HTTP client holds no connection
// state of its own, so what is reported is whether the health call got
// through.
const (
	StateReachable   = "REACHABLE"
	StateUnreachable = "UNREACHABLE"
)

// healthCheck asks the dependency behind client for its health and reports
// it under address.
func healthCheck(ctx context.Context, client *connect.Client, address string) (*diagpb.ServiceDependency, error) {
	serving, err := health.Check(ctx, client)

	state := StateReachable
	if err != nil {
		state = StateUnreachable
	}

	return &diagpb.ServiceDependency{
		Address:     address,
		Serving:     serving.String(),
		State:       state,
		LastChecked: time.Now().Unix(),
		Details:     map[string]string{},
	}, err
}

// NewDependencyCheck wires up checking a client the caller already holds.
// address is what the client was built against, reported as is: an HTTP
// client does not know its own target.
func NewDependencyCheck(client *connect.Client, address string) Check {
	return func(ctx context.Context) (*diagpb.ServiceDependency, error) {
		return healthCheck(ctx, client, address)
	}
}

// Addressed is what a check needs of something that knows which replica a
// client is on right now.
type Addressed interface {
	Address() string
}

// NewUpstreamCheck wires up checking a client whose host is chosen per
// request. It reports what NewDependencyCheck reports, with the replica
// address from upstream, which is the only party that knows it.
func NewUpstreamCheck(client *connect.Client, upstream Addressed) Check {
	return func(ctx context.Context) (*diagpb.ServiceDependency, error) {
		return healthCheck(ctx, client, upstream.Address())
	}
}

// NewTargetCheck builds a fresh Connect client for address over httpClient on
// every check, for a dependency the caller does not otherwise hold a client
// to. httpClient is the caller's, typically foundationclient.NewHTTPClient(nil);
// taking it is what lets a test supply one that dials nothing.
func NewTargetCheck(httpClient connecthttp.HTTPClient, address string, opts ...connecthttp.Option) Check {
	return func(ctx context.Context) (*diagpb.ServiceDependency, error) {
		client := foundationclient.New(httpClient, foundationclient.BaseURL(address), nil, opts...)

		return healthCheck(ctx, client, address)
	}
}
