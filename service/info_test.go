package service

import (
	"testing"

	infopb "github.com/pbrpc/connect-protos/info"
)

func TestInfoServerVersion(t *testing.T) {
	srv := &infoServer{version: "1.2.3"}

	response, err := srv.Version(t.Context(), &infopb.VersionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.GetVersion() != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", response.GetVersion())
	}
}
