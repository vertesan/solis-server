package test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

func TestLiveRound5PreparationRulesScenario(t *testing.T) {
	setting := &pmaster.Setting{LiveMinimumGuaranteedAudienceAmount: 10, LiveLightFanAmountWeightPermil: 50}
	cards := []live.CardData{{Bonuses: map[penum.PhotoAbilityType]int64{penum.PhotoAbilityType_PhotoAbilityType_AudienceAmountUp: 10}}, {}}
	for _, tc := range []struct {
		capacity int32
		want     []int32
	}{{100, []int32{20, 10}}, {15, []int32{10, 5}}} {
		got, err := live.AllocateAudience(cards, tc.capacity, setting)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("audience=%v want%v: %v", got, tc.want, err)
		}
	}
	cards[0].Bonuses[penum.PhotoAbilityType_PhotoAbilityType_AudienceAmountUp] = math.MaxInt64
	if _, err := live.AllocateAudience(cards, 100, setting); err == nil {
		t.Fatal("audience bonus overflow accepted")
	}

	in := liveEngineFixture(1)
	for _, tc := range []struct {
		kind   penum.PhotoAbilityTargetType
		source int32
		want   []int32
	}{
		{penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Center, 1, []int32{1}},
		{penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Neighbor, 3, []int32{1, 5}},
		{penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Neighbor, 1, []int32{2, 3}},
		{penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Self, 4, []int32{4}},
		{penum.PhotoAbilityTargetType_PhotoAbilityTargetType_All, 4, []int32{1, 2, 3, 4, 5}},
	} {
		got, err := live.PhotoTargetIndexes(in.Users[0].GetUserDeck().GetCards(), in.Cards, tc.source, &pmaster.PhotoAbilityTarget{Type: tc.kind})
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("photo target %v = %v want%v: %v", tc.kind, got, tc.want, err)
		}
	}
	advantage := &pmaster.QuestCharacterAdvantage{CharacterIds: []string{"matching"}, AdvantagePermil: 1250}
	if got, err := live.ResolveCharacterAdvantage(live.CardData{CharacterID: "other"}, advantage); err != nil || got != 1000 {
		t.Fatal("unmatched character received bonus", err)
	}
	if got, err := live.ResolveCharacterAdvantage(live.CardData{CharacterID: "matching"}, advantage); err != nil || got != 1250 {
		t.Fatal("character score advantage was not resolved", err)
	}
	advantage.CharacterIds, advantage.CharacterGroupId = nil, "group"
	if got, err := live.ResolveCharacterAdvantage(live.CardData{GroupID: "group"}, advantage); err != nil || got != 1250 {
		t.Fatal("group advantage was not applied", err)
	}
	owned := []string{"i", "a", "g", "h", "b", "f", "e", "d", "c"}
	displayed := []string{"a", "b", "c", "d", "e"}
	for _, tc := range []struct {
		count int32
		want  []string
	}{{5, []string{"f"}}, {6, []string{"f"}}, {8, []string{"f", "g", "h"}}, {9, []string{"f", "g", "h", "i"}}} {
		got, err := live.ExtraCharacterIDs(tc.count, owned, displayed)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("extra characters count%d=%v want%v: %v", tc.count, got, tc.want, err)
		}
	}
	if _, err := live.ExtraCharacterIDs(10, owned, displayed); err == nil {
		t.Fatal("insufficient distinct owned performers accepted")
	}
}

// Exercise the actual RPC adapter, not merely a handcrafted engine input: photo
// recipient resolution and restricted modifiers must survive master lookup.
func TestQuestRound5PhotoPreparationScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	cardIDs := []string{"card-ai-03-schl-00", "card-aoi-03-schl-00", "card-chs-03-schl-00", "card-hrk-03-schl-00", "card-ktn-03-schl-00"}
	photoIDs := [][]string{
		{"pab-cool_time_reduction_skill_1_recipe-001-cd-support", "pab-affect_strength_effect_value_increase-beat_score_up_recipe_distribution-5-cd-support"},
		{"pab-cool_time_reduction_skill_2_recipe-001-cd-appeal", "pab-received_strength_effect_value_increase-active_score_multiplier_add_recipe_distribution-5-cd-appeal"},
		{"pab-cool_time_reduction_skill_3_recipe-001-cd-technique", "pab-dance_multiply_recipe-5-pa-target-neighbor-cd-technique"},
		{"pab-affect_strength_effect_count_increase-skill_success_rate_up_recipe_distribution-5"},
		{"pab-affect01_strength_effect_count_increase-skill_dance_boost_recipe_distribution-5"},
	}
	var positions []*ptransaction.UserDeckPosition
	for i, id := range cardIDs {
		card, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, id)
		if err != nil {
			t.Fatal(err)
		}
		card.Vocal, card.Dance, card.Visual = 1000000, 1000000, 1000000
		card.BaseVocal, card.BaseDance, card.BaseVisual = card.Vocal, card.Dance, card.Visual
		card.PhotoEquipableCount, card.BasePhotoEquipableCount = 1, 1
		if err := opuser.UpsertUserCard(ctx, testConfig.TestUserSuid, card); err != nil {
			t.Fatal(err)
		}
		photo := &ptransaction.UserPhoto{PhotoId: fmt.Sprintf("round5-preparation-%d", i), Level: 250, Rarity: 5}
		for _, abilityID := range photoIDs[i] {
			definition, err := opmaster.FindPhotoAbility(ctx, abilityID)
			if err != nil {
				t.Fatal(err)
			}
			if len(definition.GetPhotoAbilityGrades()) == 0 {
				t.Fatal("fixture photo master has no grades")
			}
			photo.Abilities = append(photo.Abilities, &ptransaction.UserPhotoAbility{PhotoAbilityId: abilityID, Grade: definition.GetPhotoAbilityGrades()[0].GetGrade(), IsAvailable: true})
		}
		if err := opuser.UpsertUserPhoto(ctx, testConfig.TestUserSuid, photo); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, &ptransaction.UserDeckPosition{Position: int32(i + 1), CardId: id, PhotoIds: []string{photo.GetPhotoId()}})
	}
	saved := profileCall(t, papi.NewDeckClient(testConfig.GrpcConn).Save, &papi.DeckSaveRequest{Name: "フォト効果検証", UserDeckPositions: positions})
	_, q := questStartDailyFixture(t)
	response := profileCall(t, papi.NewQuestClient(testConfig.GrpcConn).Start, &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: saved.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber(), PlayCount: 1, IsSkip: true})
	assertQuestStartTrace(t, response)
	prepared := response.GetResult().GetUserInfos()[0].GetUserDeck().GetCards()
	if len(prepared) != 5 {
		t.Fatal("photo preparation changed scoring actor count")
	}
	for i, card := range prepared {
		if i == 0 || i == 4 {
			if card.GetDance() <= 1000000 {
				t.Fatalf("neighbor %d did not receive targeted dance photo", i+1)
			}
		} else if card.GetDance() != 1000000 {
			t.Fatalf("non-neighbor %d unexpectedly received targeted dance photo: %d", i+1, card.GetDance())
		}
	}
}
