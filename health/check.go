package health

import (
	"context"

	"connectrpc.com/connect/v2"

	healthpb "git.sonicoriginal.software/grpc-connect-protos/health"
	"git.sonicoriginal.software/grpc-connect-protos/health/healthconnect"
)

// Check asks the peer behind client whether its process is serving, through
// its grpc.health.v1.Health service. UNKNOWN comes back with the error when
// the peer could not be asked.
func Check(ctx context.Context, client *connect.Client) (healthpb.HealthCheckResponse_ServingStatus, error) {
	response, err := healthconnect.NewHealthClient(client).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return healthpb.HealthCheckResponse_UNKNOWN, err
	}

	return response.GetStatus(), nil
}
