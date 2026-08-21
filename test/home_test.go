package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestHomeSetCharacterPosition(t *testing.T) {
	setupTestUser(t)

	characters, err := opuser.FindAllUserCharacters(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) < 2 {
		t.Fatalf("test user has %d characters, want at least 2", len(characters))
	}

	requested := []*papi.HomeCharacterPosition{
		{
			Type:              penum.HomePositionType_HomePositionType_Activity,
			CharacterId:       characters[1].GetCharacterId(),
			IsCharacterRandom: false,
			IsCostumeRandom:   false,
		},
		{
			Type:              penum.HomePositionType_HomePositionType_Home,
			CharacterId:       characters[0].GetCharacterId(),
			IsCharacterRandom: false,
			IsCostumeRandom:   true,
		},
		{
			Type:              penum.HomePositionType_HomePositionType_Training,
			IsCharacterRandom: true,
			IsCostumeRandom:   true,
		},
	}
	expected := userHomePositionsFromAPI(requested)
	client := papi.NewHomeClient(testConfig.GrpcConn)
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.SetCharacterPosition(ctx, &papi.HomeSetCharacterPositionRequest{Positions: requested})
	cancel()
	if err != nil {
		t.Fatalf("Home.SetCharacterPosition failed: %v", err)
	}
	if response.GetCommonResponse() == nil || response.GetCommonResponse().GetUpdatedData() == nil {
		t.Fatalf("Home.SetCharacterPosition returned no cache delta: %v", response)
	}
	assertHomePositions(t, "response", response.GetCommonResponse().GetUpdatedData().GetHomePositions(), expected)

	ctx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	userResponse, err := papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("User.Get failed: %v", err)
	}
	assertHomePositions(t, "User.Get", userResponse.GetHomePositions(), expected)

	stored, err := opuser.FindAllUserHomePositions(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertHomePositions(t, "database", stored, expected)

	rejected := []*papi.HomeCharacterPosition{
		{Type: penum.HomePositionType_HomePositionType_Home, CharacterId: "char-not-owned"},
		{Type: penum.HomePositionType_HomePositionType_Training, IsCharacterRandom: true},
		{Type: penum.HomePositionType_HomePositionType_Activity, CharacterId: characters[1].GetCharacterId()},
	}
	ctx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SetCharacterPosition(ctx, &papi.HomeSetCharacterPositionRequest{Positions: rejected})
	cancel()
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unowned character returned %v, want PermissionDenied", err)
	}
	storedAfterReject, err := opuser.FindAllUserHomePositions(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertHomePositions(t, "database after rejected request", storedAfterReject, expected)
}

func userHomePositionsFromAPI(positions []*papi.HomeCharacterPosition) []*ptransaction.UserHomePosition {
	result := make([]*ptransaction.UserHomePosition, 0, len(positions))
	for _, position := range positions {
		result = append(result, &ptransaction.UserHomePosition{
			HomePositionType:  position.GetType(),
			CharacterId:       position.GetCharacterId(),
			IsCharacterRandom: position.GetIsCharacterRandom(),
			IsCostumeRandom:   position.GetIsCostumeRandom(),
		})
	}
	return result
}

func assertHomePositions(
	t *testing.T,
	source string,
	actual []*ptransaction.UserHomePosition,
	expected []*ptransaction.UserHomePosition,
) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("%s returned %d home positions, want %d: %v", source, len(actual), len(expected), actual)
	}
	expectedByType := make(map[penum.HomePositionType]*ptransaction.UserHomePosition, len(expected))
	for _, position := range expected {
		expectedByType[position.GetHomePositionType()] = position
	}
	seen := make(map[penum.HomePositionType]struct{}, len(actual))
	for _, position := range actual {
		positionType := position.GetHomePositionType()
		if _, duplicate := seen[positionType]; duplicate {
			t.Fatalf("%s returned duplicate home position type %s: %v", source, positionType, actual)
		}
		seen[positionType] = struct{}{}
		if !proto.Equal(position, expectedByType[positionType]) {
			t.Fatalf("%s returned unexpected position %v, want %v", source, position, expectedByType[positionType])
		}
	}
}
