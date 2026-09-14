package health

import "testing"

const exampleService = "example.ExampleService"

// status fails the test unless srv records want for service.
func status(t *testing.T, srv *Server, service string, want Status) {
	t.Helper()

	got, found := srv.Status(service)
	if !found {
		t.Fatalf("%q has no status, want %v", service, want)
	}
	if got != want {
		t.Errorf("%q status = %v, want %v", service, got, want)
	}
}

func TestStatus(t *testing.T) {
	t.Run("reports the process serving from the start", func(t *testing.T) {
		status(t, NewServer(), "", StatusServing)
	})

	t.Run("reports a service as recorded", func(t *testing.T) {
		srv := NewServer()
		srv.SetServingStatus(exampleService, StatusNotServing)

		status(t, srv, exampleService, StatusNotServing)
	})

	t.Run("reports a service never recorded as not found", func(t *testing.T) {
		if _, found := NewServer().Status(exampleService); found {
			t.Fatal("expected no status")
		}
	})
}

func TestShutdown(t *testing.T) {
	srv := NewServer()
	srv.SetServingStatus(exampleService, StatusServing)

	srv.Shutdown()

	status(t, srv, "", StatusNotServing)
	status(t, srv, exampleService, StatusNotServing)

	// Nothing comes back up after a shutdown.
	srv.SetServingStatus(exampleService, StatusServing)
	status(t, srv, exampleService, StatusNotServing)
}
