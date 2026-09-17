//revive:disable:package-comments
package service

import (
	"iter"
	"slices"
	"strings"

	"connectrpc.com/connect/v2"
)

// infrastructurePrefixes name the services Register excludes from the returned
// method list and never reports a health status for. They are up exactly when
// the process is, which the health service's own "" entry already reports.
var infrastructurePrefixes = []string{
	"grpc.",        // grpc.health.v1 and other standard gRPC services
	"info.",        // info endpoint
	"diagnostics.", // diagnostics endpoint
}

// serviceName reports the fully qualified service owning a method named in
// wire format, "/package.Service/Method".
func serviceName(method string) string {
	name, _, _ := strings.Cut(strings.TrimPrefix(method, "/"), "/")

	return name
}

// isInfrastructure reports whether service is one Register mounts itself.
func isInfrastructure(service string) bool {
	for _, prefix := range infrastructurePrefixes {
		if strings.HasPrefix(service, prefix) {
			return true
		}
	}

	return false
}

// methods reports the fully qualified names of the registered procedures that
// belong to the caller, sorted, which is what the server exposes beyond the
// infrastructure.
func methods(specs iter.Seq[connect.Spec]) []string {
	names := []string{}

	for spec := range specs {
		if isInfrastructure(serviceName(spec.Procedure)) {
			continue
		}

		names = append(names, spec.Procedure)
	}

	slices.Sort(names)

	return names
}

// serviceNames reports the distinct services owning the given fully qualified
// method names, in the order the methods appear.
func serviceNames(methods []string) []string {
	seen := map[string]struct{}{}
	names := []string{}

	for _, method := range methods {
		name := serviceName(method)
		if _, found := seen[name]; found {
			continue
		}

		seen[name] = struct{}{}
		names = append(names, name)
	}

	return names
}
