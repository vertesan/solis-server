package test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

// A complete configured-user flow with a real god-sized owned inventory.
// Setup, User.Get and assertions are deliberately outside the timed RPC.
// Host load varies, so elapsed time is reported rather than a brittle threshold.
func TestQuestStartGodInventoryPerformanceScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	ctx := context.Background()
	// God mode grants ownership, not card training. Unlock actual level-gated
	// abilities on the large outside-deck inventory before taking the oracle
	// snapshot, instead of fabricating ability IDs or weakening the assertion.
	ids := []string{"card-skr-05-idol-00", "card-szk-03-schl-00", "card-rei-03-schl-00", "card-hrk-04-casl-00", "card-chs-04-casl-00"}
	trained := trainQuestPerformanceInventory(t, ids)
	userClient := papi.NewUserClient(testConfig.GrpcConn)
	before := profileCall(t, userClient.Get, &papi.Empty{})
	byCard := map[string]*ptransaction.UserCard{}
	for _, card := range before.GetCards() {
		byCard[card.GetCardId()] = card
	}
	// These old, ordinary cards have validated native families. No unsupported
	// photo recipes/equipment are needed to exercise the global-owned-card scan.
	var positions []*ptransaction.UserDeckPosition
	selected := map[string]bool{}
	for i, id := range ids {
		if byCard[id] == nil {
			t.Fatalf("god fixture lacks master card %s", id)
		}
		selected[id] = true
		positions = append(positions, &ptransaction.UserDeckPosition{Position: int32(i + 1), CardId: id})
	}
	if len(before.GetCards()) <= len(ids) {
		t.Fatal("god fixture does not exercise a large owned inventory")
	}
	saved := profileCall(t, papi.NewDeckClient(testConfig.GrpcConn).Save, &papi.DeckSaveRequest{Name: "所持全体の性能検証", UserDeckPositions: positions})
	number := saved.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber()
	area, q := questStartDailyFixture(t)
	items, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ticketID := ""
	for _, item := range items {
		if item.GetType() == area.GetDailyTicketItemType() {
			ticketID = item.GetId()
			break
		}
	}
	if ticketID == "" || q.GetTicketAmount() <= 0 {
		t.Fatal("daily fixture lacks a usable pass definition")
	}
	// Exhaust FREE allowance and supply an exact owned-pass balance, independent
	// of god mode's resource policy and the current day's previously used count.
	questSeedArea(t, area, int32(len(area.GetQuestIds())), 5, time.Now())
	initialPasses := int64(q.GetTicketAmount()) + 7
	setCardTrainingItem(t, ticketID, initialPasses)

	// Independently reconstruct the Vocal equation from User.Get's real owned
	// state, master abilities and staff level. At least one contributor MUST be
	// outside this five-card unit, preventing an accidental deck-only shortcut.
	abilityIDs := map[string]bool{}
	for _, card := range before.GetCards() {
		if card.GetLiveAbilityId() != "" {
			abilityIDs[card.GetLiveAbilityId()] = true
		}
	}
	abilities, err := opmaster.FindLiveAbilities(ctx, coverageKeys(abilityIDs))
	if err != nil {
		t.Fatal(err)
	}
	byAbility := map[string]*pmaster.LiveAbility{}
	for _, ability := range abilities {
		byAbility[ability.GetId()] = ability
	}
	var globalRatio, globalFlat int64
	outsideContributors := 0
	for _, card := range before.GetCards() {
		for _, level := range byAbility[card.GetLiveAbilityId()].GetLevels() {
			if level.GetLevel() != card.GetLiveAbilityLevel() || level.GetRequiredCardLevel() > card.GetLevel() || level.GetMoodType() != penum.MoodType_MoodType_Unknown && level.GetMoodType() != q.GetMoodType() {
				continue
			}
			contributes := false
			switch level.GetType() {
			case penum.LiveAbilityType_LiveAbilityType_VocalMultiply:
				globalRatio += int64(level.GetValue())
				contributes = level.GetValue() > 0
			case penum.LiveAbilityType_LiveAbilityType_VocalAdd:
				globalFlat += int64(level.GetValue())
				contributes = level.GetValue() > 0
			}
			if contributes && !selected[card.GetCardId()] {
				outsideContributors++
			}
		}
	}
	if outsideContributors == 0 {
		t.Fatal("god fixture has no outside-deck vocal live ability")
	}
	staffLevels, err := opmaster.FindAllStaffLevels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var staffFlat int64
	for _, staff := range before.GetStaffs() {
		if staff.GetParameterType() != penum.ParameterType_ParameterType_Vocal {
			continue
		}
		found := staff.GetLevel() == 0
		for _, level := range staffLevels {
			if level.GetParameterType() == staff.GetParameterType() && level.GetLevel() == staff.GetLevel() {
				staffFlat = int64(level.GetAdvantage())
				found = true
				break
			}
		}
		if !found {
			t.Fatal("god staff level is missing from master")
		}
	}
	companyRatio := map[string]int64{}
	for _, character := range before.GetCharacters() {
		for _, bonus := range character.GetCompanyBonusParameterInfos() {
			if bonus.GetBonusParameterType() == penum.ParameterType_ParameterType_Vocal {
				companyRatio[character.GetCharacterId()] += bonus.GetBonusParameterUpPermil()
			}
		}
	}
	masters, err := opmaster.FindLiveCards(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	masterByID := map[string]*pmaster.Card{}
	for _, card := range masters {
		masterByID[card.GetId()] = card
	}

	rpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	started := time.Now()
	response, err := papi.NewQuestClient(testConfig.GrpcConn).Start(rpcCtx, &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 1, UseDailyTicket: true, IsSkip: true})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("god-inventory Quest.Start: ownedCards=%d trainedOutsideDeck=%d photos=%d accessories=%d outsideDeckVocalContributors=%d elapsed=%s responseProtobufBytes=%d sourceUserGetProtobufBytes=%d", len(before.GetCards()), trained, len(before.GetPhotos()), len(before.GetAccessories()), outsideContributors, elapsed, proto.Size(response), proto.Size(before))
	if !response.GetResult().GetCleared() {
		t.Fatal("god daily fixture did not clear")
	}
	assertQuestStartTrace(t, response)
	if err := checkLiveScoreConservation(response.GetResult()); err != nil {
		t.Fatal(err)
	}
	if got := questStartUsablePasses(t, ticketID); got != initialPasses-int64(q.GetTicketAmount()) {
		t.Fatalf("paid daily consumed %d passes, want %d", initialPasses-got, q.GetTicketAmount())
	}
	for _, prepared := range response.GetResult().GetUserInfos()[0].GetUserDeck().GetCards() {
		base := byCard[prepared.GetCardId()]
		master := masterByID[prepared.GetCardId()]
		if base == nil || master == nil {
			t.Fatal("prepared card differs from selected owned unit")
		}
		expected := base.GetVocal()*(1000+globalRatio+companyRatio[master.GetCharacterId()])/1000 + globalFlat + staffFlat
		if prepared.GetVocal() != expected {
			t.Fatalf("prepared %s vocal=%d want %d from base=%d allOwnedRatio=%d", prepared.GetCardId(), prepared.GetVocal(), expected, base.GetVocal(), globalRatio)
		}
	}
	after := profileCall(t, userClient.Get, &papi.Empty{})
	checked := 0
	for _, card := range after.GetCards() {
		if !selected[card.GetCardId()] {
			continue
		}
		checked++
		base := byCard[card.GetCardId()]
		if card.GetLevel() == base.GetLevel() && card.GetRarity() == base.GetRarity() {
			if card.GetVocal() != base.GetVocal() || card.GetDance() != base.GetDance() || card.GetVisual() != base.GetVisual() || card.GetStamina() != base.GetStamina() {
				t.Fatal("live preparation wrote transient bonuses into persistent UserCard")
			}
		}
		if card.GetBaseLevel() == base.GetBaseLevel() && card.GetRarity() == base.GetRarity() {
			if card.GetBaseVocal() != base.GetBaseVocal() || card.GetBaseDance() != base.GetBaseDance() || card.GetBaseVisual() != base.GetBaseVisual() || card.GetBaseStamina() != base.GetBaseStamina() {
				t.Fatal("live preparation changed persistent unassisted card parameters")
			}
		}
	}
	if checked != len(ids) {
		t.Fatal("User.Get lost a selected card after the live")
	}
}

// Fixture construction only: use the same master-based attribute and release
// calculators as Card.Enhance, including coherent EXP and preserved base fields.
// The configured test user is deleted by setupTestUser's cleanup. No production
// god-mode behavior or master data is changed, and this work is not timed.
func trainQuestPerformanceInventory(t *testing.T, selected []string) int {
	t.Helper()
	ctx := context.Background()
	const targetLevel int32 = 30
	level, err := opmaster.FindCardLevel(ctx, targetLevel)
	if err != nil {
		t.Fatal(err)
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := opmaster.FindManagerLevel(ctx, user.GetManagerLevel())
	if err != nil || manager.GetMaxCardLevel() < targetLevel {
		t.Fatalf("god fixture cannot train to level %d: %v", targetLevel, err)
	}
	masters, err := opmaster.FindAllCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byMaster := make(map[string]*pmaster.Card, len(masters))
	for _, master := range masters {
		byMaster[master.GetId()] = master
	}
	owned, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var trained []*ptransaction.UserCard
	for _, card := range owned {
		master := byMaster[card.GetCardId()]
		if master == nil || master.GetLiveAbilityId() == "" || slices.Contains(selected, card.GetCardId()) || card.GetBaseLevel() >= targetLevel {
			continue
		}
		if card.GetSupported() {
			t.Fatalf("fresh god fixture unexpectedly assists %s", card.GetCardId())
		}
		rarity, err := opmaster.FindCardRarity(ctx, int64(card.GetRarity()))
		if err != nil || rarity.GetLevelLimit() < targetLevel {
			t.Fatalf("fixture card %s cannot reach level %d: %v", card.GetCardId(), targetLevel, err)
		}
		attributes, err := usercard.CalcUserCardBaseAttr(ctx, master, targetLevel, card.GetRarity())
		if err != nil {
			t.Fatal(err)
		}
		photos, skills, abilities, err := usercard.CalcUnlocksWithEvolution(ctx, targetLevel, card.GetEvolutionLevel(), master)
		if err != nil {
			t.Fatal(err)
		}
		card.TotalExp, card.Level, card.BaseLevel = level.GetRequiredExp(), targetLevel, targetLevel
		card.Vocal, card.Dance, card.Visual, card.Stamina = attributes.Vocal, attributes.Dance, attributes.Visual, attributes.Stamina
		card.BaseVocal, card.BaseDance, card.BaseVisual, card.BaseStamina = attributes.Vocal, attributes.Dance, attributes.Visual, attributes.Stamina
		card.PhotoEquipableCount, card.BasePhotoEquipableCount = photos, photos
		card.SkillId1, card.SkillId2, card.SkillId3, card.SkillId4 = skills[0], skills[1], skills[2], skills[3]
		card.BaseSkillId1, card.BaseSkillId2, card.BaseSkillId3, card.BaseSkillId4 = skills[0], skills[1], skills[2], skills[3]
		card.SkillLevel1, card.SkillLevel2, card.SkillLevel3, card.SkillLevel4 = 1, 1, 1, 1
		card.BaseSkillLevel1, card.BaseSkillLevel2, card.BaseSkillLevel3, card.BaseSkillLevel4 = 1, 1, 1, 1
		card.LiveAbilityId, card.ActivityAbilityId = abilities[0], abilities[1]
		card.BaseLiveAbilityId, card.BaseActivityAbilityId = abilities[0], abilities[1]
		card.LiveAbilityLevel, card.ActivityAbilityLevel = 1, 1
		card.BaseLiveAbilityLevel, card.BaseActivityAbilityLevel = 1, 1
		card.LastEnhanceTime = time.Now().UnixMilli()
		trained = append(trained, card)
	}
	if len(trained) == 0 {
		t.Fatal("master fixture has no outside-deck cards to train")
	}
	if err := opuser.UpsertManyUserCards(ctx, testConfig.TestUserSuid, trained); err != nil {
		t.Fatal(err)
	}
	return len(trained)
}
