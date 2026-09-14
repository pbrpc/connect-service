//revive:disable:package-comments
package service

import (
	"fmt"

	"connectrpc.com/connect/v2"

	foundation "git.sonicoriginal.software/connect-foundation/server"
	"git.sonicoriginal.software/grpc-connect-protos/diagnostics/diagnosticsconnect"
	"git.sonicoriginal.software/grpc-connect-protos/health/healthconnect"
	"git.sonicoriginal.software/grpc-connect-protos/info/infoconnect"
	healthpb "git.sonicoriginal.software/grpc-connect-protos/health"

	"git.sonicoriginal.software/connect-service/diagnostics"
	"git.sonicoriginal.software/connect-service/health"
)

// Register attaches the caller's services to rpc, along with the diagnostics,
// info, and health services every service exposes, and puts the health
// service's plain HTTP route on mux. It marks each of the caller's services
// SERVING and returns their fully qualified method names, which is what the
// server exposes beyond that infrastructure.
//
// checks are the dependencies the diagnostics service reports on. Every
// dependency is the caller's to name; nil means none.
//
// Register only assembles. Mounting the procedures, serving, shutdown, and
// any later health status change are the caller's to make.
func Register(
	rpc *connect.Server,
	mux Mux,
	healthSrv Health,
	checks diagnostics.Checks,
	registerFn func(*connect.Server),
) ([]string, error) {
	if rpc == nil {
		return nil, fmt.Errorf("rpc cannot be nil")
	}

	if mux == nil {
		return nil, fmt.Errorf("mux cannot be nil")
	}

	if healthSrv == nil {
		return nil, fmt.Errorf("healthSrv cannot be nil")
	}

	if registerFn != nil {
		registerFn(rpc)
	}

	diagnosticsconnect.RegisterDiagnosticsServiceHandler(rpc, diagnostics.NewServer(checks))
	infoconnect.RegisterInfoServiceHandler(rpc, &infoServer{version: foundation.Version()})
	healthconnect.RegisterHealthHandler(rpc, healthSrv)
	mux.Handle(health.HTTPPath, healthSrv)

	methodNames := methods(rpc.Specs())

	for _, serviceName := range serviceNames(methodNames) {
		healthSrv.SetServingStatus(serviceName, healthpb.HealthCheckResponse_SERVING)
	}

	return methodNames, nil
}
