//revive:disable:package-comments
package service

import (
	"net/http"

	healthpb "git.sonicoriginal.software/grpc-connect-protos/health"
	"git.sonicoriginal.software/grpc-connect-protos/health/healthconnect"
)

// Health is the health service the caller builds and owns. Register attaches
// it, as procedures and as the plain HTTP route, and marks the caller's own
// services SERVING; every status change after that is the caller's, since
// only the application knows what a degraded dependency means for it.
//
// *health.Server satisfies this. A caller keeps the handle to flip a service
// to NOT_SERVING, or calls its Shutdown to drain before stopping.
type Health interface {
	healthconnect.HealthHandler
	http.Handler

	SetServingStatus(service string, status healthpb.HealthCheckResponse_ServingStatus)
}
