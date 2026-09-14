# connect-service

Endpoints every Connect server should expose: health, info, and dependency
diagnostics, assembled in one call.

## About

[connect-foundation](https://github.com/sonic-original-software/connect-foundation)
decides how a server serves: its options, telemetry, errors, and shutdown. This
library decides what a server exposes beyond its own services. A caller that
wants the first without the second takes foundation alone.

The contract a consumer signs up for are the protos in
[grpc-connect-protos](https://github.com/sonic-original-software/grpc-connect-protos)
plus the mounted set:

- `grpc.health.v1.Health` — the standard health service, as procedures and as
  `GET /healthz` for probes that can only GET
- `info.InfoService` — the server's version, read from `GRPC_SERVER_VERSION`
- `diagnostics.DiagnosticsService` — the state of each dependency the caller
  names

Every procedure answers gRPC, gRPC-Web, and Connect-protocol clients.

## Installation

```bash
go get git.sonicoriginal.software/connect-service
```

## What's Included

- **`service/`** — `Register`, which attaches the three services above
  alongside the caller's own
- **`health/`** — the health server and the client-side `Check`
- **`diagnostics/`** — the diagnostics server and the checks it runs

## Usage

### Assembly

`service.Register` attaches the caller's services to the RPC dispatcher, then
the three infrastructure services, puts the health route on the mux, marks
each of the caller's services SERVING, and returns their fully qualified method
names.

It assembles only. The listener, the server, the background goroutines, and the
blocking `Serve` call are the caller's, because those are the pieces that differ
between production and a test.

`service/example_test.go` holds the whole sequence as a Go `Example`. It has no
`// Output:` comment, so `go test` compiles it and never runs it, which keeps it
type-checked against the real API. `service/assembly_test.go` runs the same
assembly in-process and calls every mounted endpoint. Read those rather than a
copy here.

The returned method list excludes the `grpc.`, `info.`, and `diagnostics.`
services, which are infrastructure rather than something a caller advertises.

### Health Status

`health.NewServer()` marks the `""` entry SERVING, which answers "is this
process alive". `Register` adds an entry per service the caller registered,
under its fully qualified name, so a probe asks about
`yourpackage.YourService` rather than a logical name of your choosing.

Those entries start SERVING and stay there until you change them. Only your
application knows whether a given upstream being down means it can still do its
job, so deciding that is yours:

```go
healthSrv.SetServingStatus(
    yourpbconnect.YourServiceName,
    healthpb.HealthCheckResponse_NOT_SERVING,
)
```

The generated `YourServiceName` constant is the same name `Register` used, so
the two cannot drift.

The same statuses answer over plain HTTP: `GET /healthz` for the process and
`GET /healthz?service=yourpackage.YourService` for one service, 200 for
SERVING, 503 otherwise, 404 for a service never recorded, with the
`HealthCheckResponse` as the JSON body.

### Diagnostics

`diagnostics.Checks` maps a dependency name to a `Check`. `Register` mounts a
diagnostics server over the map; `GetDiagnostics` runs every check concurrently
and reports each dependency's address, health, and reachability under its name.
`state` is `REACHABLE` when the dependency answered the health call and
`UNREACHABLE` when it did not.

Three constructors cover the usual cases:

- `NewDependencyCheck(client, address)` reports on a client the caller already
  holds, under the address it was built against. Prefer this: what is reported
  is what is in use.
- `NewUpstreamCheck(client, upstream)` is `NewDependencyCheck` for a client
  whose host is chosen per request. `upstream` supplies the replica address the
  client is on right now.
- `NewTargetCheck(httpClient, address)` builds a fresh client on every check,
  for a dependency the caller does not otherwise hold a client to.

A `Check` is a plain function, so a dependency that is not a Connect peer — a
database, a cache — is one the caller writes.

## Configuration

- `GRPC_SERVER_VERSION` — what the info service answers with. Read through
  `connect-foundation`.
