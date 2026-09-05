package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestMigrationSetPasswordScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewMigrationClient(testConfig.GrpcConn)
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	basicCtx, basicCancel := getBasicGrpcCtx()
	defer basicCancel()
	dbCtx := context.Background()
	before, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	oldTransfer, err := client.MigratePassword(basicCtx, &papi.MigrationMigratePasswordRequest{
		UserId: testConfig.TestUserPublicId, Password: testConfig.TestUserPass,
	})
	if err != nil {
		t.Fatal(err)
	}
	newPassword := testConfig.TestUserPass + "-changed"
	response, err := client.SetPassword(ctx, &papi.MigrationSetPasswordRequest{Password: newPassword})
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(response.GetCommonResponse().GetUpdatedData().GetUser(), before) {
		t.Fatal("SetPassword changed the game user or omitted its cache delta")
	}
	auth, err := opuser.GetUserAuthByServerId(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if auth.Password != newPassword || auth.OnetimeToken != "" {
		t.Fatal("new password or transfer-token invalidation was not persisted")
	}
	methods, err := client.List(ctx, &papi.Empty{})
	if err != nil || !methods.GetPassword() {
		t.Fatalf("Migration.List omitted password availability: %v", err)
	}
	if _, err := client.Execute(basicCtx, &papi.MigrationExecuteRequest{OnetimeToken: oldTransfer.GetOnetimeToken()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("old transfer token returned %v", err)
	}
	if _, err := client.MigratePassword(basicCtx, &papi.MigrationMigratePasswordRequest{
		UserId: testConfig.TestUserPublicId, Password: testConfig.TestUserPass,
	}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("old password returned %v", err)
	}
	if _, err := client.SetPassword(ctx, &papi.MigrationSetPasswordRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty password returned %v", err)
	}
	newTransfer, err := client.MigratePassword(basicCtx, &papi.MigrationMigratePasswordRequest{
		UserId: testConfig.TestUserPublicId, Password: newPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	if newTransfer.GetLinkedUserId() != testConfig.TestUserPublicId || newTransfer.GetOnetimeToken() == "" {
		t.Fatal("new password did not authorize transfer of the configured user")
	}
	if result, err := client.Execute(basicCtx, &papi.MigrationExecuteRequest{OnetimeToken: newTransfer.GetOnetimeToken()}); err != nil || result.GetToken() == "" {
		t.Fatalf("transfer with new password failed: %v", err)
	}
	// Accounts without password migration enabled must not accept empty passwords.
	if err := opuser.UpdateUserAuthPassword(dbCtx, testConfig.TestUserSuid, ""); err != nil {
		t.Fatal(err)
	}
	if methods, err := client.List(ctx, &papi.Empty{}); err != nil || methods.GetPassword() {
		t.Fatalf("password-disabled account advertised password migration: %v", err)
	}
	if _, err := client.MigratePassword(basicCtx, &papi.MigrationMigratePasswordRequest{UserId: testConfig.TestUserPublicId}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("empty password authenticated a password-disabled account: %v", err)
	}
	if _, err := client.SetPassword(ctx, &papi.MigrationSetPasswordRequest{Password: testConfig.TestUserPass}); err != nil {
		t.Fatal(err)
	}
	if methods, err := client.List(ctx, &papi.Empty{}); err != nil || !methods.GetPassword() {
		t.Fatalf("setting a password did not enable password migration: %v", err)
	}
}
