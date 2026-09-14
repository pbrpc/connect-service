package service

import (
	"slices"
	"testing"

	"connectrpc.com/connect/v2"
)

func TestServiceName(t *testing.T) {
	cases := map[string]string{
		"/example.ExampleService/Create": exampleService,
		"example.ExampleService/Create":  exampleService,
		"/example.ExampleService/":       exampleService,
		"/example.ExampleService":        exampleService,
		"":                               "",
	}

	for procedure, want := range cases {
		t.Run(procedure, func(t *testing.T) {
			if got := serviceName(procedure); got != want {
				t.Errorf("serviceName = %q, want %q", got, want)
			}
		})
	}
}

func TestIsInfrastructure(t *testing.T) {
	cases := map[string]bool{
		"grpc.health.v1.Health":          true,
		"info.InfoService":               true,
		"diagnostics.DiagnosticsService": true,
		exampleService:                   false,
		"":                               false,
	}

	for service, want := range cases {
		t.Run(service, func(t *testing.T) {
			if got := isInfrastructure(service); got != want {
				t.Errorf("isInfrastructure = %v, want %v", got, want)
			}
		})
	}
}

func TestMethods(t *testing.T) {
	specs := []connect.Spec{
		{Procedure: "/example.ExampleService/Delete"},
		{Procedure: "/grpc.health.v1.Health/Check"},
		{Procedure: "/example.ExampleService/Create"},
		{Procedure: "/info.InfoService/Version"},
	}

	got := methods(slices.Values(specs))

	want := []string{"/example.ExampleService/Create", "/example.ExampleService/Delete"}
	if !slices.Equal(got, want) {
		t.Errorf("methods = %v, want %v", got, want)
	}
}

func TestServiceNames(t *testing.T) {
	got := serviceNames([]string{
		"/example.ExampleService/Create",
		"/other.OtherService/Get",
		"/example.ExampleService/Delete",
	})

	want := []string{exampleService, "other.OtherService"}
	if !slices.Equal(got, want) {
		t.Errorf("serviceNames = %v, want %v", got, want)
	}
}
