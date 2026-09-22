package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestQuestAssetScenario(t *testing.T) {
	setupTestUser(t)
	fixture := prepareDeckFixture(t)
	ctx := context.Background()
	deckClient := papi.NewDeckClient(testConfig.GrpcConn)
	client := papi.NewQuestClient(testConfig.GrpcConn)
	positions := []*ptransaction.UserDeckPosition{
		{Position: 5, CardId: fixture.cards[2].GetId()},
		{Position: 3, CardId: fixture.cards[1].GetId()},
		{Position: 1, CardId: fixture.cards[0].GetId(), CostumeId: fixture.costume.GetId(), HairId: fixture.hairID},
	}
	saved := profileCall(t, deckClient.Save, &papi.DeckSaveRequest{Name: "アセット", UserDeckPositions: positions})
	number := saved.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber()
	before, err := opuser.FindUserDeckPositions(ctx, testConfig.TestUserSuid, number)
	if err != nil {
		t.Fatal(err)
	}
	request := &papi.QuestListAssetRequest{QuestId: "qt-main-1-003-01", DeckNumber: number}
	response := profileCall(t, client.ListAsset, request)
	assets := response.GetCharacterAssets()
	if len(assets) != 3 {
		t.Fatalf("partial unit returned %d assets instead of its three occupied slots", len(assets))
	}
	if assets[0].GetCharacterId() != fixture.cards[0].GetCharacterId() || assets[0].GetCostumeId() != fixture.costume.GetId() || assets[0].GetHairId() != fixture.hairID {
		t.Fatalf("first occupied slot did not preserve its saved appearance: %v", assets[0])
	}
	if assets[2].GetCharacterId() != fixture.cards[2].GetCharacterId() {
		t.Fatal("replacement idol stole a later occupied slot's original idol")
	}
	seen := make(map[string]bool)
	for _, asset := range assets {
		if asset.GetCharacterId() == "" || asset.GetCostumeId() == "" || asset.GetHairId() == "" || seen[asset.GetCharacterId()] {
			t.Fatalf("live appearance is empty or duplicated: %v", asset)
		}
		seen[asset.GetCharacterId()] = true
		if _, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, asset.GetCharacterId()); err != nil {
			t.Fatalf("live substitution used an unowned idol: %v", err)
		}
	}
	if proto.Size(response.GetCommonResponse().GetUpdatedData()) != 0 || proto.Size(response.GetCommonResponse().GetDeletedData()) != 0 {
		t.Fatal("asset prefetch unexpectedly mutated client state")
	}
	repeated := profileCall(t, client.ListAsset, request)
	if !proto.Equal(response, repeated) {
		t.Fatal("repeated prefetch changed the deterministic live appearance")
	}
	// Dream uses this same endpoint without a master quest ID. It still uses
	// the saved deck and must preserve partial-unit appearance semantics.
	withoutQuest := &papi.QuestListAssetRequest{DeckNumber: number}
	if dreamAssets := profileCall(t, client.ListAsset, withoutQuest); !proto.Equal(response, dreamAssets) {
		t.Fatal("quest-independent prefetch changed the partial deck's appearances")
	}
	after, err := opuser.FindUserDeckPositions(ctx, testConfig.TestUserSuid, number)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("asset prefetch changed saved deck size")
	}
	for index := range before {
		if !proto.Equal(before[index], after[index]) {
			t.Fatal("visual substitution was incorrectly persisted into the unit")
		}
	}

	// Without a deck-specific appearance, the user's live settings apply.
	profileCall(t, deckClient.Save, &papi.DeckSaveRequest{Number: number, Name: "ライブ衣装", UserDeckPositions: []*ptransaction.UserDeckPosition{{Position: 1, CardId: fixture.cards[0].GetId()}}})
	fallback := profileCall(t, client.ListAsset, request).GetCharacterAssets()
	if len(fallback) != 1 || fallback[0].GetCostumeId() != fixture.costume.GetId() || fallback[0].GetHairId() != fixture.hairID {
		t.Fatal("unspecified deck appearance did not use the user's live settings")
	}
	profileReject(t, codes.InvalidArgument, client.ListAsset, &papi.QuestListAssetRequest{QuestId: request.GetQuestId()})
	profileReject(t, codes.InvalidArgument, client.ListAsset, &papi.QuestListAssetRequest{})
	profileReject(t, codes.InvalidArgument, client.ListAsset, &papi.QuestListAssetRequest{DeckNumber: -1})
	profileReject(t, codes.NotFound, client.ListAsset, &papi.QuestListAssetRequest{QuestId: "quest-does-not-exist", DeckNumber: number})
	profileReject(t, codes.FailedPrecondition, client.ListAsset, &papi.QuestListAssetRequest{QuestId: request.GetQuestId(), DeckNumber: number, UserBuddyDeckPositions: positions})
	profileReject(t, codes.FailedPrecondition, client.ListAsset, &papi.QuestListAssetRequest{DeckNumber: number, UserBuddyDeckPositions: positions})

	// Empty units may be prefetched; live eligibility belongs to Quest.Start.
	profileCall(t, deckClient.Save, &papi.DeckSaveRequest{Number: number, Name: "空"})
	if assets := profileCall(t, client.ListAsset, request).GetCharacterAssets(); len(assets) != 0 {
		t.Fatal("empty deck returned unrelated characters")
	}
	if assets := profileCall(t, client.ListAsset, withoutQuest).GetCharacterAssets(); len(assets) != 0 {
		t.Fatal("quest-independent prefetch returned characters for an empty deck")
	}
	profileCall(t, deckClient.Delete, &papi.DeckDeleteRequest{Number: number})
	profileReject(t, codes.NotFound, client.ListAsset, request)
	profileReject(t, codes.NotFound, client.ListAsset, withoutQuest)
}
