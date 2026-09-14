//revive:disable:package-comments
package service

import "net/http"

// Mux is what this package needs of the caller's HTTP mux: somewhere to put
// the plain HTTP routes that sit beside the procedures. *http.ServeMux
// satisfies it; a test satisfies it with a recorder.
//
// The procedures themselves go on the *connect.Server directly: v2's
// dispatcher is concrete, and its Specs is the report of what was registered.
type Mux interface {
	Handle(pattern string, handler http.Handler)
}
