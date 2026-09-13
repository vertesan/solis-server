package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestPhotoDeleteEquippedScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	suid := testConfig.TestUserSuid
	photoClient := papi.NewPhotoClient(testConfig.GrpcConn)
	userClient := papi.NewUserClient(testConfig.GrpcConn)
	levels, err := opmaster.FindAllPhotoRarityLevels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var level *pmaster.PhotoRarityLevel
	for _, candidate := range levels {
		if candidate.GetRarity() > 0 && candidate.GetLevel() > 0 && candidate.GetDeleteRewardItem() > 0 {
			level = candidate
			break
		}
	}
	if level == nil {
		t.Fatal("master data has no deletable photo quality")
	}
	const keepA, keepB = "deck-photo-keep-a", "deck-photo-keep-b"
	const soldA, soldB = "deck-photo-sold-a", "deck-photo-sold-b"
	for _, id := range []string{keepA, keepB, soldA, soldB} {
		if err := opuser.UpsertUserPhoto(ctx, suid, &ptransaction.UserPhoto{
			PhotoId: id, Rarity: level.GetRarity(), Level: level.GetLevel(), Locked: id == soldB,
		}); err != nil {
			t.Fatal(err)
		}
	}
	positions, err := opuser.FindAllUserDeckPositions(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) < 3 {
		t.Fatal("configured user needs at least three initial deck positions")
	}
	positions[0].PhotoIds = []string{keepA, soldA, keepB, soldB}
	positions[1].PhotoIds = []string{soldA, soldB}
	positions[2].PhotoIds = []string{keepA}
	otherDeck := proto.Clone(positions[0]).(*ptransaction.UserDeckPosition)
	otherDeck.Number = positions[len(positions)-1].GetNumber() + 1
	otherDeck.PhotoIds = []string{soldB}
	positions = append(positions, otherDeck)
	if err := opuser.UpsertUserDeck(ctx, suid, &ptransaction.UserDeck{Number: otherDeck.Number, Name: "Photo equipment scenario"}); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertManyUserDeckPositions(ctx, suid, positions); err != nil {
		t.Fatal(err)
	}
	leaguePositions := []*ptransaction.UserLeagueDeckPosition{
		{
			LeagueSeasonId: "photo-delete-scenario", DeckType: penum.LeagueDeckType_LeagueDeckType_A, Position: 1,
			CardId: positions[0].GetCardId(), CostumeId: positions[0].GetCostumeId(), HairId: positions[0].GetHairId(),
			PhotoIds: []string{soldB, keepB, soldA, keepA},
		},
		{
			LeagueSeasonId: "photo-delete-scenario", DeckType: penum.LeagueDeckType_LeagueDeckType_B, Position: 1,
			CardId: positions[1].GetCardId(), PhotoIds: []string{keepB},
		},
	}
	if err := opuser.UpsertManyUserLeagueDeckPositions(ctx, suid, leaguePositions); err != nil {
		t.Fatal(err)
	}
	const recycleItemID = "item-photo_reroll"
	recycleBefore := photoPermanentItemAmount(t, ctx, recycleItemID)
	request := &papi.PhotoDeleteRequest{PhotoIds: []string{soldA, soldB}}

	// A later locked photo rejects the whole request, including equipment and rewards.
	profileReject(t, codes.FailedPrecondition, photoClient.Delete, request)
	get := profileCall(t, userClient.Get, &papi.Empty{})
	assertPhotoDeleteDeckPositions(t, get.GetDeckPositions(), positions)
	assertPhotoDeleteLeaguePositions(t, get.GetLeagueDeckPositions(), leaguePositions)
	if photoWithID(get.GetPhotos(), soldA) == nil || photoWithID(get.GetPhotos(), soldB) == nil ||
		photoPermanentItemAmount(t, ctx, recycleItemID) != recycleBefore {
		t.Fatal("rejected photo sale changed photos or recycle film")
	}
	profileCall(t, photoClient.SwitchLock, &papi.PhotoSwitchLockRequest{PhotoIds: []string{soldB}})
	response := profileCall(t, photoClient.Delete, request)
	updated := response.GetCommonResponse().GetUpdatedData()
	positions[0].PhotoIds = []string{keepA, keepB}
	positions[1].PhotoIds = nil
	otherDeck.PhotoIds = nil
	leaguePositions[0].PhotoIds = []string{keepB, keepA}
	assertPhotoDeleteDeckPositions(t, updated.GetDeckPositions(), []*ptransaction.UserDeckPosition{positions[0], positions[1], otherDeck})
	assertPhotoDeleteLeaguePositions(t, updated.GetLeagueDeckPositions(), leaguePositions[:1])
	deleted := response.GetCommonResponse().GetDeletedData().GetPhotos()
	if len(deleted) != 2 || photoWithID(deleted, soldA) == nil || photoWithID(deleted, soldB) == nil {
		t.Fatalf("photo sale omitted deleted photo identities: %v", deleted)
	}
	wantReward := 2 * int64(level.GetDeleteRewardItem())
	if delta := photoPermanentItemAmount(t, ctx, recycleItemID) - recycleBefore; delta != wantReward {
		t.Fatalf("sale recycle film delta = %d, want %d", delta, wantReward)
	}
	var reportedReward int64
	for _, reward := range response.GetRewardResults() {
		if reward.GetResourceType() == penum.ResourceType_ResourceType_Item && reward.GetResourceId() == recycleItemID {
			reportedReward += reward.GetAmountI64()
		}
	}
	if reportedReward != wantReward {
		t.Fatalf("response recycle reward = %d, want %d", reportedReward, wantReward)
	}
	get = profileCall(t, userClient.Get, &papi.Empty{})
	assertPhotoDeleteDeckPositions(t, get.GetDeckPositions(), positions)
	assertPhotoDeleteLeaguePositions(t, get.GetLeagueDeckPositions(), leaguePositions)
	if photoWithID(get.GetPhotos(), soldA) != nil || photoWithID(get.GetPhotos(), soldB) != nil ||
		photoWithID(get.GetPhotos(), keepA) == nil || photoWithID(get.GetPhotos(), keepB) == nil {
		t.Fatal("User.Get did not preserve unsold photos and remove sold photos")
	}
}

func assertPhotoDeleteDeckPositions(t *testing.T, got, want []*ptransaction.UserDeckPosition) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("deck position count = %d, want %d", len(got), len(want))
	}
	byKey := make(map[[2]int32]*ptransaction.UserDeckPosition, len(got))
	for _, position := range got {
		byKey[[2]int32{position.GetNumber(), position.GetPosition()}] = position
	}
	for _, position := range want {
		actual := byKey[[2]int32{position.GetNumber(), position.GetPosition()}]
		if !proto.Equal(actual, position) {
			t.Fatalf("deck position changed unexpectedly: got=%v want=%v", actual, position)
		}
	}
}

func assertPhotoDeleteLeaguePositions(t *testing.T, got, want []*ptransaction.UserLeagueDeckPosition) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("league position count = %d, want %d", len(got), len(want))
	}
	key := func(position *ptransaction.UserLeagueDeckPosition) string {
		return fmt.Sprintf("%s:%d:%d", position.GetLeagueSeasonId(), position.GetDeckType(), position.GetPosition())
	}
	byKey := make(map[string]*ptransaction.UserLeagueDeckPosition, len(got))
	for _, position := range got {
		byKey[key(position)] = position
	}
	for _, position := range want {
		if actual := byKey[key(position)]; !proto.Equal(actual, position) {
			t.Fatalf("league position changed unexpectedly: got=%v want=%v", actual, position)
		}
	}
}
