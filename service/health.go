//revive:disable:package-comments
package service

import (
	"net/http"

	"github.com/pbrpc/connect-service/health"
)

// Health is the health service the caller builds and owns. Register mounts it
// on the probe route and marks the caller's own services SERVING; every
// status change after that is the caller's, since only the application knows
// what a degraded dependency means for it.
//
// *health.Server satisfies this. A caller keeps the handle to flip a service
// to NOT_SERVING, or calls its Shutdown to drain before stopping.
type Health interface {
	http.Handler

	SetServingStatus(service string, status health.Status)
}
