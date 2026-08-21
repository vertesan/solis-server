package test

import (
	"testing"

	"github.com/vertesan/solis-server/internal/proto/papi"
)

func TestMigrationList(t *testing.T) {
	setupTestUser(t)

	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	response, err := papi.NewMigrationClient(testConfig.GrpcConn).List(ctx, &papi.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetCommonResponse() == nil {
		t.Fatal("migration list omitted common response")
	}
	if !response.GetGoogle() || !response.GetPassword() {
		t.Fatalf("configured test user's migration methods are missing: %v", response)
	}
	if response.GetApple() || response.GetFacebook() || response.GetTwitter() {
		t.Fatalf("migration list advertised unsupported methods: %v", response)
	}
}
