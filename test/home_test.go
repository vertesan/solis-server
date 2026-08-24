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
	ownedCharacterIDs := make(map[string]struct{}, len(characters))
	for _, character := range characters {
		ownedCharacterIDs[character.GetCharacterId()] = struct{}{}
	}

	requested := []*papi.HomeCharacterPosition{
		{
			Type:              penum.HomePositionType_HomePositionType_Activity,
			IsCharacterRandom: true,
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
	expected := response.GetCommonResponse().GetUpdatedData().GetHomePositions()
	if len(expected) != len(requested) {
		t.Fatalf("response returned %d home positions, want %d: %v", len(expected), len(requested), expected)
	}
	requestedByType := make(map[penum.HomePositionType]*papi.HomeCharacterPosition, len(requested))
	for _, position := range requested {
		requestedByType[position.GetType()] = position
	}
	seenTypes := make(map[penum.HomePositionType]struct{}, len(expected))
	for _, position := range expected {
		positionType := position.GetHomePositionType()
		requestedPosition, requestedType := requestedByType[positionType]
		if !requestedType {
			t.Fatalf("response returned unknown position type %s: %v", positionType, expected)
		}
		if _, duplicate := seenTypes[positionType]; duplicate {
			t.Fatalf("response returned duplicate position type %s: %v", positionType, expected)
		}
		seenTypes[positionType] = struct{}{}
		if position.GetCharacterId() == "" {
			t.Fatalf("response returned an empty character for position %s: %v", positionType, expected)
		}
		if _, owned := ownedCharacterIDs[position.GetCharacterId()]; !owned {
			t.Fatalf("response selected unowned character %q: %v", position.GetCharacterId(), expected)
		}
		if position.GetIsCharacterRandom() != requestedPosition.GetIsCharacterRandom() ||
			position.GetIsCostumeRandom() != requestedPosition.GetIsCostumeRandom() {
			t.Fatalf("response changed random settings for position %s: request=%v response=%v",
				positionType, requestedPosition, position)
		}
		if !requestedPosition.GetIsCharacterRandom() && position.GetCharacterId() != requestedPosition.GetCharacterId() {
			t.Fatalf("response changed fixed character for position %s: request=%v response=%v",
				positionType, requestedPosition, position)
		}
	}

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
