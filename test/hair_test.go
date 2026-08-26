package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestHairScenario(t *testing.T) {
	setupTestUser(t)

	hairs, err := opuser.FindAllUserHairs(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(hairs) == 0 {
		t.Fatal("test user has no hair")
	}
	hair := hairs[0]
	hair.Checked = false
	if err := opuser.UpsertUserHair(context.Background(), testConfig.TestUserSuid, hair); err != nil {
		t.Fatal(err)
	}

	client := papi.NewHairClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.CheckHair(grpcCtx, &papi.HairCheckRequest{HairId: hair.GetHairId()})
	cancel()
	if err != nil {
		t.Fatalf("Hair.CheckHair failed: %v", err)
	}
	updatedHairs := response.GetCommonResponse().GetUpdatedData().GetHairs()
	if len(updatedHairs) != 1 || updatedHairs[0].GetHairId() != hair.GetHairId() || !updatedHairs[0].GetChecked() {
		t.Fatalf("Hair.CheckHair returned the wrong hair post-image: %v", response)
	}
	persisted, err := opuser.FindUserHair(context.Background(), testConfig.TestUserSuid, hair.GetHairId())
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.GetChecked() {
		t.Fatalf("Hair.CheckHair did not persist the checked state: %v", persisted)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := client.CheckHair(grpcCtx, &papi.HairCheckRequest{HairId: hair.GetHairId()})
	cancel()
	if err != nil {
		t.Fatalf("repeated Hair.CheckHair failed: %v", err)
	}
	if repeated.GetCommonResponse() == nil || len(repeated.GetCommonResponse().GetUpdatedData().GetHairs()) != 0 {
		t.Fatalf("repeated Hair.CheckHair returned a redundant update: %v", repeated)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CheckHair(grpcCtx, &papi.HairCheckRequest{})
	cancel()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty Hair.CheckHair error is %v, want InvalidArgument", err)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CheckHair(grpcCtx, &papi.HairCheckRequest{HairId: "hair-test-missing"})
	cancel()
	if status.Code(err) != codes.NotFound {
		t.Fatalf("unowned Hair.CheckHair error is %v, want NotFound", err)
	}
}
