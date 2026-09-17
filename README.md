# connect-service

Endpoints every Connect server should expose: health, info, and dependency
diagnostics, assembled in one call.

## About

This library decides what a server exposes beyond its own services. A caller
that wants the first without the second takes foundation alone.

The contract a consumer signs up for are the protos in
[connect-protos](https://github.com/pbrpc/connect-protos) plus the mounted set:

- `GET /healthz` — the serving status of the process and of each service, for
  HTTP probes
- `info.InfoService` — the server's version, read from `SERVICE_VERSION`
- `diagnostics.DiagnosticsService` — the state of each dependency the caller
  names

Every procedure answers gRPC, gRPC-Web, and Connect-protocol clients.

## Installation

```bash
go get github.com/pbrpc/connect-service
```

## What's Included

- **`service/`** — `Register`, which attaches the endpoints above alongside the
  caller's own
- **`health/`** — the health server behind `GET /healthz` and the client-side
  `Check` that probes it
- **`diagnostics/`** — the diagnostics server and the checks it runs

## Usage

### Assembly

`service.Register` attaches the caller's services to the RPC dispatcher, then
the info and diagnostics services, puts the health route on the mux, marks each
of the caller's services SERVING, and returns their fully qualified method
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
under its fully qualified name, so a probe asks about `yourpackage.YourService`
rather than a logical name of your choosing.

Those entries start SERVING and stay there until you change them. Only your
application knows whether a given upstream being down means it can still do its
job, so deciding that is yours:

```go
healthSrv.SetServingStatus(yourpbconnect.YourServiceName, health.StatusNotServing)
```

The generated `YourServiceName` constant is the same name `Register` used, so
the two cannot drift.

The statuses answer on `GET /healthz` for the process and
`GET /healthz?service=yourpackage.YourService` for one service: 200 for SERVING,
503 otherwise, 404 for a service never recorded, with `{"status":"SERVING"}` as
the JSON body. Kubernetes `httpGet` probes, load balancer health checks, and
`health.Check` all read it.

### Diagnostics

`diagnostics.Checks` maps a dependency name to a `Check`. `Register` mounts a
diagnostics server over the map; `GetDiagnostics` runs every check concurrently
and reports each dependency's address, health, and reachability under its name.
`state` is `REACHABLE` when the dependency answered the health probe and
`UNREACHABLE` when it did not.

Two constructors cover the usual cases:

- `NewDependencyCheck(httpClient, address)` probes the dependency at `address`
  over `httpClient`, the same HTTP client the caller's Connect client for that
  dependency is built on, so what is reported is what is in use.
- `NewUpstreamCheck(httpClient, upstream)` is `NewDependencyCheck` for a
  dependency whose replica is chosen per request. `upstream` supplies the
  address it is on right now.

A `Check` is a plain function, so a dependency that is not a Connect peer — a
database, a cache — is one the caller writes.

## Configuration

- `SERVICE_VERSION` — what the info service answers with. Read from `service`.
