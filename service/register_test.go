package service

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect/v2"

	"github.com/pbrpc/connect-service/health"
)

func TestRegister(t *testing.T) {
	t.Run("returns an error when rpc is nil", func(t *testing.T) {
		_, err := Register(nil, &muxStub{}, newHealthStub(), nil, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "rpc cannot be nil") {
			t.Errorf("error = %q, want it to mention a nil rpc", err)
		}
	})

	t.Run("returns an error when mux is nil", func(t *testing.T) {
		_, err := Register(connect.NewServer(), nil, newHealthStub(), nil, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "mux cannot be nil") {
			t.Errorf("error = %q, want it to mention a nil mux", err)
		}
	})

	t.Run("returns an error when healthSrv is nil", func(t *testing.T) {
		_, err := Register(connect.NewServer(), &muxStub{}, nil, nil, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "healthSrv cannot be nil") {
			t.Errorf("error = %q, want it to mention a nil healthSrv", err)
		}
	})

	t.Run("registers the caller's services", func(t *testing.T) {
		rpc := connect.NewServer()

		if _, err := Register(rpc, &muxStub{}, newHealthStub(), nil, registerExampleService); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !registered(rpc, exampleService) {
			t.Error("the caller's service was not registered")
		}
	})

	t.Run("registers diagnostics and info", func(t *testing.T) {
		rpc := connect.NewServer()

		if _, err := Register(rpc, &muxStub{}, newHealthStub(), nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, name := range []string{
			"diagnostics.DiagnosticsService",
			"info.InfoService",
		} {
			if !registered(rpc, name) {
				t.Errorf("%q was not registered", name)
			}
		}
	})

	t.Run("mounts the health route", func(t *testing.T) {
		mux := &muxStub{}

		if _, err := Register(connect.NewServer(), mux, newHealthStub(), nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !mux.handled(health.HTTPPath) {
			t.Errorf("routes = %v, want %q", mux.patterns, health.HTTPPath)
		}
	})

	t.Run("accepts a nil checks map", func(t *testing.T) {
		rpc := connect.NewServer()

		if _, err := Register(rpc, &muxStub{}, newHealthStub(), nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !registered(rpc, "diagnostics.DiagnosticsService") {
			t.Error("the diagnostics service was not registered")
		}
	})

	t.Run("accepts a nil registerFn", func(t *testing.T) {
		rpc := connect.NewServer()

		if _, err := Register(rpc, &muxStub{}, newHealthStub(), nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if registered(rpc, exampleService) {
			t.Error("no caller service should be registered")
		}
	})

	t.Run("returns the caller's methods and excludes infrastructure", func(t *testing.T) {
		got, err := Register(connect.NewServer(), &muxStub{}, newHealthStub(), nil, registerExampleService)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{
			"/example.ExampleService/Create",
			"/example.ExampleService/Delete",
		}
		if !slices.Equal(got, want) {
			t.Errorf("methods = %v, want %v", got, want)
		}
	})

	t.Run("marks the caller's services serving, once each", func(t *testing.T) {
		healthSrv := newHealthStub()

		if _, err := Register(connect.NewServer(), &muxStub{}, healthSrv, nil, registerExampleService); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := map[string]health.Status{
			exampleService: health.StatusServing,
		}
		if len(healthSrv.statuses) != len(want) {
			t.Fatalf("statuses = %v, want %v", healthSrv.statuses, want)
		}
		for name, status := range want {
			if healthSrv.statuses[name] != status {
				t.Errorf("%q status = %v, want %v", name, healthSrv.statuses[name], status)
			}
		}
	})

	t.Run("reports no serving status for infrastructure services", func(t *testing.T) {
		healthSrv := newHealthStub()

		if _, err := Register(connect.NewServer(), &muxStub{}, healthSrv, nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(healthSrv.statuses) != 0 {
			t.Errorf("statuses = %v, want none", healthSrv.statuses)
		}
	})
}
