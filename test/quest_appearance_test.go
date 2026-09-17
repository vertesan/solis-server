package test

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	usercostume "github.com/vertesan/solis-server/internal/makino/user/costume"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

func TestQuestAssetStartAppearanceScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	// Keep the starter's owned-character subset: all-master replacement selection
	// would incorrectly choose an unowned collab idol before the correct substitute.
	duplicate, err := usercard.SpawnUserCardById(ctx, "card-ai-02-eve-00")
	if err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserCard(ctx, testConfig.TestUserSuid, duplicate); err != nil {
		t.Fatal(err)
	}
	ids := []string{"card-ai-03-schl-00", duplicate.GetCardId(), "card-aoi-03-schl-00", "card-chs-03-schl-00", "card-hrk-03-schl-00"}
	var positions []*ptransaction.UserDeckPosition
	reserved := map[string]bool{}
	for i, id := range ids {
		card, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, id)
		if err != nil {
			t.Fatal(err)
		}
		master, err := opmaster.FindCard(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		reserved[master.GetCharacterId()] = true
		card.Vocal, card.Dance, card.Visual = 1000000, 1000000, 1000000
		card.BaseVocal, card.BaseDance, card.BaseVisual = card.Vocal, card.Dance, card.Visual
		if err := opuser.UpsertUserCard(ctx, testConfig.TestUserSuid, card); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, &ptransaction.UserDeckPosition{Position: int32(i + 1), CardId: id})
	}
	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(characters, func(i, j int) bool { return characters[i].GetCharacterId() < characters[j].GetCharacterId() })
	var replacement *ptransaction.UserCharacter
	owned := map[string]bool{}
	for _, character := range characters {
		owned[character.GetCharacterId()] = true
		if replacement == nil && !reserved[character.GetCharacterId()] {
			replacement = character
		}
	}
	if replacement == nil {
		t.Fatal("starter fixture lacks an owned replacement")
	}
	masterReplacement, err := opmaster.FindCharacter(ctx, replacement.GetCharacterId())
	if err != nil {
		t.Fatal(err)
	}
	masterCharacters, err := opmaster.FindAllCharacters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	unownedExists := false
	for _, character := range masterCharacters {
		if !owned[character.GetId()] && !reserved[character.GetId()] && character.GetId() < replacement.GetCharacterId() {
			unownedExists = true
			break
		}
	}
	if !unownedExists {
		t.Fatal("master fixture no longer distinguishes owned from all-master replacement order")
	}
	costumes, err := opmaster.FindAllCostumes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	hairs, err := opmaster.FindAllHairs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var costume *pmaster.Costume
	var incompatible *pmaster.Hair
	for _, candidate := range costumes {
		if candidate.GetCharacterId() != replacement.GetCharacterId() || candidate.GetId() == masterReplacement.GetDefaultCostumeId() || candidate.GetId() == masterReplacement.GetDefaultLiveCostumeId() || candidate.GetDefaultHairId() == "" || candidate.GetIsUserUnavailable() || strings.HasPrefix(candidate.GetCostumeTypeId(), "sd-") {
			continue
		}
		for _, hair := range hairs {
			if hair.GetCharacterId() != replacement.GetCharacterId() || hair.GetId() == candidate.GetDefaultHairId() {
				continue
			}
			if slices.Contains(hair.GetNotWearableCostumeIds(), candidate.GetId()) || len(hair.GetWearableCostumeIds()) > 0 && !slices.Contains(hair.GetWearableCostumeIds(), candidate.GetId()) {
				costume, incompatible = candidate, hair
				break
			}
		}
		if costume != nil {
			break
		}
	}
	if costume == nil || incompatible == nil {
		t.Fatal("master lacks a custom replacement costume/incompatible global hair pair")
	}
	if err := opuser.UpsertUserCostume(ctx, testConfig.TestUserSuid, usercostume.SpawnUserCostume(costume)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{costume.GetDefaultHairId(), incompatible.GetId()} {
		if err := opuser.UpsertUserHair(ctx, testConfig.TestUserSuid, &ptransaction.UserHair{HairId: id, Checked: true}); err != nil {
			t.Fatal(err)
		}
	}
	replacement.LiveCostumeId, replacement.LiveHairId = costume.GetId(), incompatible.GetId()
	if err := opuser.UpsertUserCharacter(ctx, testConfig.TestUserSuid, replacement); err != nil {
		t.Fatal(err)
	}
	// Blank live settings also exercise the master default-live/default fallback.
	original, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, "char-ai")
	if err != nil {
		t.Fatal(err)
	}
	original.LiveCostumeId, original.LiveHairId = "", ""
	if err := opuser.UpsertUserCharacter(ctx, testConfig.TestUserSuid, original); err != nil {
		t.Fatal(err)
	}
	saved := profileCall(t, papi.NewDeckClient(testConfig.GrpcConn).Save, &papi.DeckSaveRequest{Name: "外見の整合性", UserDeckPositions: positions})
	number := saved.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber()
	before, err := opuser.FindUserDeckPositions(ctx, testConfig.TestUserSuid, number)
	if err != nil {
		t.Fatal(err)
	}
	_, quest := questStartDailyFixture(t)
	client := papi.NewQuestClient(testConfig.GrpcConn)
	listed := profileCall(t, client.ListAsset, &papi.QuestListAssetRequest{QuestId: quest.GetId(), DeckNumber: number})
	if len(listed.GetCharacterAssets()) != 5 {
		t.Fatal("prefetch did not return five actors")
	}
	substitute := listed.GetCharacterAssets()[1]
	if substitute.GetCharacterId() != replacement.GetCharacterId() || substitute.GetCostumeId() != costume.GetId() || substitute.GetHairId() != costume.GetDefaultHairId() {
		t.Fatalf("prefetch substitution/fallback mismatch: %v", substitute)
	}
	displayed := map[string]bool{}
	for _, asset := range listed.GetCharacterAssets() {
		displayed[asset.GetCharacterId()] = true
	}
	var spareBefore *ptransaction.UserCharacter
	for _, character := range characters {
		if !displayed[character.GetCharacterId()] {
			spareBefore = character
			break
		}
	}
	if spareBefore == nil {
		t.Fatal("appearance fixture lacks a spare cosmetic idol")
	}
	started := profileCall(t, client.Start, &papi.QuestStartRequest{QuestId: quest.GetId(), DeckNumber: number, PlayCount: 1, IsSkip: true})
	assertQuestStartTrace(t, started)
	extras := started.GetResult().GetUserInfos()[0].GetUserDeck().GetExtraCharacterInfos()
	if len(extras) != 1 || extras[0].GetPosition() != 6 || extras[0].GetCharacterId() != spareBefore.GetCharacterId() || displayed[extras[0].GetCharacterId()] || extras[0].GetCostumeId() == "" || extras[0].GetHairId() == "" {
		t.Fatalf("invalid cosmetic spare: %v", extras)
	}
	if _, err := opuser.FindUserCostume(ctx, testConfig.TestUserSuid, extras[0].GetCostumeId()); err != nil {
		t.Fatal(err)
	}
	if _, err := opuser.FindUserHair(ctx, testConfig.TestUserSuid, extras[0].GetHairId()); err != nil {
		t.Fatal(err)
	}
	if len(started.GetResult().GetPreChart().GetCardStatuses()) != 5 || len(started.GetResult().GetUserInfos()[0].GetUserResult().GetCardResults()) != 5 {
		t.Fatal("cosmetic spare was added as a scoring actor")
	}
	for _, chart := range started.GetResult().GetCharts() {
		for _, beat := range chart.GetBeats() {
			if beat.GetCardIndex() < 1 || beat.GetCardIndex() > 5 {
				t.Fatal("cosmetic spare received beat score")
			}
		}
	}
	spareAfter, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, spareBefore.GetCharacterId())
	if err != nil {
		t.Fatal(err)
	}
	if spareAfter.GetLightFanAmount() != spareBefore.GetLightFanAmount() || spareAfter.GetMiddleFanAmount() != spareBefore.GetMiddleFanAmount() || spareAfter.GetHeavyFanAmount() != spareBefore.GetHeavyFanAmount() {
		t.Fatal("cosmetic spare received participating-idol fan rewards")
	}
	for i, card := range started.GetResult().GetUserInfos()[0].GetUserDeck().GetCards() {
		asset := listed.GetCharacterAssets()[i]
		if card.GetDisplayCharacterId() != asset.GetCharacterId() || card.GetDisplayCostumeId() != asset.GetCostumeId() || card.GetDisplayHairId() != asset.GetHairId() {
			t.Fatalf("slot%d Start differs from prefetched assets: %s/%s/%s versus %v", i+1, card.GetDisplayCharacterId(), card.GetDisplayCostumeId(), card.GetDisplayHairId(), asset)
		}
		if card.GetCardId() != ids[i] {
			t.Fatal("visual substitution changed the scoring card")
		}
	}
	after, err := opuser.FindUserDeckPositions(ctx, testConfig.TestUserSuid, number)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("live changed deck positions")
	}
	for i, p := range before {
		if !proto.Equal(p, after[i]) {
			t.Fatal("visual substitution was persisted into the saved unit")
		}
	}
	repeated := profileCall(t, client.ListAsset, &papi.QuestListAssetRequest{QuestId: quest.GetId(), DeckNumber: number})
	if !proto.Equal(listed.GetCharacterAssets()[1], repeated.GetCharacterAssets()[1]) {
		t.Fatal("reading/starting live changed the replacement's projected appearance")
	}
}
