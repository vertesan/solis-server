package test

import (
	"context"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestCardAssistProgressionScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	suid := testConfig.TestUserSuid
	client := papi.NewCardClient(testConfig.GrpcConn)
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user.ManagerLevel = manager.Level
	if err := opuser.UpsertUser(ctx, suid, user); err != nil {
		t.Fatal(err)
	}
	trainers, err := opuser.FindAllUserCards(ctx, suid)
	if err != nil || len(trainers) < 5 {
		t.Fatalf("initial trainer cards: %v, %v", trainers, err)
	}
	trainers = trainers[:5]
	ownedCharacters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	owned := make(map[string]bool)
	for _, character := range ownedCharacters {
		owned[character.CharacterId] = true
	}
	masters, err := opmaster.FindAllCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var master *pmaster.Card
	for _, candidate := range masters {
		if owned[candidate.CharacterId] && candidate.InitialRarity <= 2 {
			master = candidate
			break
		}
	}
	if master == nil {
		t.Fatal("no low-rarity card for an initially owned character")
	}
	assisted, err := usercard.SpawnUserCard(ctx, master)
	if err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserCard(ctx, suid, assisted); err != nil {
		t.Fatal(err)
	}
	rarity, err := opmaster.FindCardRarity(ctx, int64(assisted.Rarity))
	if err != nil {
		t.Fatal(err)
	}
	target := rarity.LevelLimit + 1
	level, err := opmaster.FindCardLevel(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	setCardTrainingItem(t, "item-card_enhance", 5*level.RequiredExp+100)
	for index, trainer := range trainers {
		goal := target
		if index == 4 {
			goal--
		}
		response := cardSupportCall(t, func(ctx context.Context) (*papi.CardEnhanceResponse, error) {
			return client.Enhance(ctx, &papi.CardEnhanceRequest{CardId: trainer.CardId, Level: goal})
		})
		current, err := opuser.FindUser(ctx, suid)
		if err != nil {
			t.Fatal(err)
		}
		want := int32(1)
		if index == 4 {
			want = target - 1
			if response.GetCommonResponse().GetUpdatedData().GetUser().GetCardSupportLevel() != want {
				t.Fatalf("fifth trainer did not publish assist level %d: %v", want, response)
			}
		}
		if current.CardSupportLevel != want {
			t.Fatalf("assist level after training card %d = %d, want %d", index+1, current.CardSupportLevel, want)
		}
	}
	cardSupportExpectCode(t, codes.FailedPrecondition, func(ctx context.Context) error {
		_, err := client.SetSupport(ctx, &papi.CardSetSupportRequest{Number: 1, CardId: trainers[0].CardId})
		return err
	})
	assigned := cardSupportCall(t, func(ctx context.Context) (*papi.CardSetSupportResponse, error) {
		return client.SetSupport(ctx, &papi.CardSetSupportRequest{Number: 1, CardId: assisted.CardId})
	})
	support := assigned.GetCommonResponse().GetUpdatedData().GetCardSupports()[0]
	if cooldown := time.Until(time.UnixMilli(support.RemovableTime)); cooldown < 12*time.Hour-time.Minute || cooldown > 12*time.Hour {
		t.Fatalf("support removal cooldown = %v, want 12 hours", cooldown)
	}
	beforeEXP := userItemAmount(t, "item-card_enhance")
	cardSupportExpectCode(t, codes.FailedPrecondition, func(ctx context.Context) error {
		_, err := client.Enhance(ctx, &papi.CardEnhanceRequest{CardId: assisted.CardId, Level: target})
		return err
	})
	if userItemAmount(t, "item-card_enhance") != beforeEXP {
		t.Fatal("rejected assisted-card enhancement consumed EXP materials")
	}
	raised := cardSupportCall(t, func(ctx context.Context) (*papi.CardEnhanceResponse, error) {
		return client.Enhance(ctx, &papi.CardEnhanceRequest{CardId: trainers[4].CardId, Level: target})
	})
	if updates := raised.GetCommonResponse().GetUpdatedData(); updates.GetUser().GetCardSupportLevel() != target || len(updates.GetCards()) != 2 {
		t.Fatalf("trainer enhancement omitted raised assistance: %v", raised)
	}
	assertCardAssistProjection(t, master, assisted, target)

	// User.Get repairs old users whose trained cards predate the support-level
	// update logic. An assisted card must not act as an extra high-level trainer.
	legacyUser, err := opuser.FindUser(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	legacyUser.CardSupportLevel = 1
	if err := opuser.UpsertUser(ctx, suid, legacyUser); err != nil {
		t.Fatal(err)
	}
	legacyCard, err := opuser.FindUserCard(ctx, suid, assisted.CardId)
	if err != nil {
		t.Fatal(err)
	}
	legacyCard.Level = 1
	if err := opuser.UpsertUserCard(ctx, suid, legacyCard); err != nil {
		t.Fatal(err)
	}
	got := cardSupportCall(t, func(ctx context.Context) (*papi.UserGetResponse, error) {
		return papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	})
	if got.GetUser().GetCardSupportLevel() != target {
		t.Fatalf("User.Get did not repair legacy assist progression: %v", got.GetUser())
	}
	foundAssisted := false
	for _, card := range got.GetCards() {
		if card.CardId == assisted.CardId {
			foundAssisted = true
			if card.Level != target || !card.Supported || card.BaseLevel != assisted.BaseLevel {
				t.Fatalf("User.Get returned a stale assisted card: %v", card)
			}
		}
	}
	if !foundAssisted {
		t.Fatal("User.Get omitted the assisted card")
	}
	if err := opuser.UpsertUserBalance(ctx, suid, &ptransaction.UserBalance{FreeBalance: 10000}); err != nil {
		t.Fatal(err)
	}
	cardSupportCall(t, func(ctx context.Context) (*papi.CardResetResponse, error) {
		return client.Reset(ctx, &papi.CardResetRequest{CardId: trainers[0].CardId})
	})
	got = cardSupportCall(t, func(ctx context.Context) (*papi.UserGetResponse, error) {
		return papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	})
	if got.GetUser().GetCardSupportLevel() != target {
		t.Fatalf("resetting a trainer lowered achieved assist level: %v", got.GetUser())
	}
	assertCardAssistProjection(t, master, assisted, target)

	// Drop the stored high-water mark to model a pre-fix user after a trainer
	// reset. Only four naturally trained cards remain; the assisted card is not
	// allowed to manufacture the missing fifth trainer.
	legacyUser = got.GetUser()
	legacyUser.CardSupportLevel = 1
	if err := opuser.UpsertUser(ctx, suid, legacyUser); err != nil {
		t.Fatal(err)
	}
	got = cardSupportCall(t, func(ctx context.Context) (*papi.UserGetResponse, error) {
		return papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	})
	if got.GetUser().GetCardSupportLevel() != 1 {
		t.Fatalf("assisted card was counted as a trainer: %v", got.GetUser())
	}
	support.RemovableTime = time.Now().Add(-time.Minute).UnixMilli()
	if err := opuser.UpsertUserCardSupport(ctx, suid, support); err != nil {
		t.Fatal(err)
	}
	removed := cardSupportCall(t, func(ctx context.Context) (*papi.CardRemoveSupportResponse, error) {
		return client.RemoveSupport(ctx, &papi.CardRemoveSupportRequest{Number: 1})
	})
	if cards := removed.GetCommonResponse().GetUpdatedData().GetCards(); len(cards) != 1 || !proto.Equal(cards[0], assisted) {
		t.Fatalf("support removal did not preserve natural training: %v", removed)
	}
}

func assertCardAssistProjection(t *testing.T, master *pmaster.Card, base *ptransaction.UserCard, level int32) {
	t.Helper()
	ctx := context.Background()
	card, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, base.CardId)
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := usercard.CalcUserCardBaseAttr(ctx, master, level, base.Rarity)
	if err != nil {
		t.Fatal(err)
	}
	photos, skills, abilities, err := usercard.CalcUnlocksWithEvolution(ctx, level, base.EvolutionLevel, master)
	if err != nil {
		t.Fatal(err)
	}
	if !card.Supported || card.Level != level || card.BaseLevel != base.BaseLevel || card.TotalExp != base.TotalExp ||
		card.BaseVocal != base.BaseVocal || card.BaseDance != base.BaseDance || card.BaseVisual != base.BaseVisual || card.BaseStamina != base.BaseStamina ||
		card.Vocal != attributes.Vocal || card.Dance != attributes.Dance || card.Visual != attributes.Visual || card.Stamina != attributes.Stamina ||
		card.PhotoEquipableCount != photos || card.SkillId1 != skills[0] || card.SkillId2 != skills[1] || card.SkillId3 != skills[2] || card.SkillId4 != skills[3] ||
		card.LiveAbilityId != abilities[0] || card.ActivityAbilityId != abilities[1] {
		t.Fatalf("incorrect assisted projection at level %d: %v", level, card)
	}
}

func TestCardSupportScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	ctx := context.Background()
	master := cardSupportScenarioMaster(t, ctx)
	card, err := usercard.SpawnUserCard(ctx, master)
	if err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserCard(ctx, testConfig.TestUserSuid, card); err != nil {
		t.Fatal(err)
	}
	for _, story := range master.Stories {
		if err := opuser.DeleteUserStory(ctx, testConfig.TestUserSuid, story.StoryId); err != nil {
			t.Fatal(err)
		}
	}
	client := papi.NewCardClient(testConfig.GrpcConn)
	beforeSkill, beforeAbility := userItemAmount(t, "item-card_skill"), userItemAmount(t, "item-card_ability")
	assigned := cardSupportCall(t, func(ctx context.Context) (*papi.CardSetSupportResponse, error) {
		return client.SetSupport(ctx, &papi.CardSetSupportRequest{CardId: card.CardId, Number: 1})
	})
	updated := assigned.GetCommonResponse().GetUpdatedData()
	if len(updated.GetCards()) != 1 || !updated.Cards[0].Supported || updated.Cards[0].BaseLevel != card.BaseLevel || updated.Cards[0].Level <= card.Level {
		t.Fatalf("SetSupport did not preserve base training and apply support: %v", assigned)
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Cards[0].Level != setting.CardSupportLevelLimit {
		t.Fatalf("god assistance did not preserve the master cap: %v", updated.Cards[0])
	}
	if len(updated.GetStories()) < 2 {
		t.Fatalf("support did not unlock card stories: %v", updated.GetStories())
	}
	readStory := proto.Clone(updated.Stories[len(updated.Stories)-1]).(*ptransaction.UserStory)
	readStory.StatusType = penum.StoryStatusType_StoryStatusType_Read
	if err := opuser.UpsertUserStory(ctx, testConfig.TestUserSuid, readStory); err != nil {
		t.Fatal(err)
	}
	cardSupportCall(t, func(ctx context.Context) (*papi.CardSkillEnhanceResponse, error) {
		return client.SkillEnhance(ctx, &papi.CardSkillEnhanceRequest{CardId: card.CardId, SkillId: master.SkillId1})
	})
	abilityEnhanced := cardSupportCall(t, func(ctx context.Context) (*papi.CardActivityAbilityEnhanceResponse, error) {
		return client.ActivityAbilityEnhance(ctx, &papi.CardActivityAbilityEnhanceRequest{CardId: card.CardId, AbilityId: master.ActivityAbilityId})
	})
	if cards := abilityEnhanced.GetCommonResponse().GetUpdatedData().GetCards(); len(cards) != 1 ||
		cards[0].ActivityAbilityLevel != card.ActivityAbilityLevel+1 || cards[0].BaseActivityAbilityLevel != card.BaseActivityAbilityLevel {
		t.Fatalf("supported activity ability enhancement changed base training or returned incorrect level: %v", abilityEnhanced)
	}
	if userItemAmount(t, "item-card_skill") >= beforeSkill || userItemAmount(t, "item-card_ability") >= beforeAbility {
		t.Fatal("support training did not consume its materials")
	}
	cardSupportExpectCode(t, codes.FailedPrecondition, func(ctx context.Context) error {
		_, err := client.RemoveSupport(ctx, &papi.CardRemoveSupportRequest{Number: 1})
		return err
	})
	cardSupportExpectCode(t, codes.InvalidArgument, func(ctx context.Context) error {
		_, err := client.UpdateDisplayType(ctx, &papi.CardUpdateDisplayTypeRequest{CardId: card.CardId, DisplayType: 99})
		return err
	})
	cardSupportExpectCode(t, codes.FailedPrecondition, func(ctx context.Context) error {
		_, err := client.UpdateDisplayType(ctx, &papi.CardUpdateDisplayTypeRequest{
			CardId: card.CardId, DisplayType: penum.CardDisplayType_CardDisplayType_AfterEvolution,
		})
		return err
	})
	displayResponse := cardSupportCall(t, func(ctx context.Context) (*papi.CardUpdateDisplayTypeResponse, error) {
		return client.UpdateDisplayType(ctx, &papi.CardUpdateDisplayTypeRequest{
			CardId: card.CardId, DisplayType: penum.CardDisplayType_CardDisplayType_AfterLimitBreak,
		})
	})
	if cards := displayResponse.GetCommonResponse().GetUpdatedData().GetCards(); len(cards) != 1 || cards[0].DisplayType != penum.CardDisplayType_CardDisplayType_AfterLimitBreak {
		t.Fatalf("UpdateDisplayType returned incorrect card: %v", displayResponse)
	}
	evolvedCards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var evolved, beforeLimitBreak *ptransaction.UserCard
	for _, candidate := range evolvedCards {
		if candidate.CardId == card.CardId {
			continue
		}
		images, err := opmaster.FindCardImages(ctx, candidate.CardId)
		if err != nil {
			t.Fatal(err)
		}
		for _, image := range images {
			if image.DisplayType == penum.CardDisplayType_CardDisplayType_AfterEvolution {
				evolved = candidate
			}
			if image.DisplayType == penum.CardDisplayType_CardDisplayType_BeforeLimitBreak {
				beforeLimitBreak = candidate
			}
		}
		if evolved != nil && beforeLimitBreak != nil {
			break
		}
	}
	if evolved == nil {
		t.Fatal("no card can exercise evolution display selection")
	}
	evolvedMaster, err := opmaster.FindCard(ctx, evolved.CardId)
	if err != nil {
		t.Fatal(err)
	}
	evolutionLevels, err := opmaster.FindCardEvolutionLevels(ctx, evolvedMaster.CardEvolutionLevelGroupId)
	if err != nil {
		t.Fatal(err)
	}
	for _, level := range evolutionLevels {
		if level.Level <= 0 {
			continue
		}
		evolutions, err := opmaster.FindCardEvolutions(ctx, evolved.CardId, level.Level-1, level.Level)
		if err != nil {
			t.Fatal(err)
		}
		if len(evolutions) == 0 {
			continue
		}
		evolved.EvolutionLevel, evolved.EvolutionLevelTotalExp = level.Level, int64(level.Exp)
		break
	}
	if evolved.EvolutionLevel <= 0 {
		t.Fatal("evolution image card has no usable CardEvolution master tier")
	}
	if err := opuser.UpsertUserCard(ctx, testConfig.TestUserSuid, evolved); err != nil {
		t.Fatal(err)
	}
	if beforeLimitBreak == nil {
		t.Fatal("no card can exercise pre-limit-break display selection")
	}
	beforeDisplay := cardSupportCall(t, func(ctx context.Context) (*papi.CardUpdateDisplayTypeResponse, error) {
		return client.UpdateDisplayType(ctx, &papi.CardUpdateDisplayTypeRequest{
			CardId: beforeLimitBreak.CardId, DisplayType: penum.CardDisplayType_CardDisplayType_BeforeLimitBreak,
		})
	})
	beforeLimitBreak.DisplayType = penum.CardDisplayType_CardDisplayType_BeforeLimitBreak
	if changed := beforeDisplay.GetCommonResponse().GetUpdatedData().GetCards(); len(changed) != 1 || !proto.Equal(changed[0], beforeLimitBreak) {
		t.Fatalf("pre-limit-break display selection changed card training or returned incorrect display: %v", beforeDisplay)
	}
	evolvedDisplay := cardSupportCall(t, func(ctx context.Context) (*papi.CardUpdateDisplayTypeResponse, error) {
		return client.UpdateDisplayType(ctx, &papi.CardUpdateDisplayTypeRequest{
			CardId: evolved.CardId, DisplayType: penum.CardDisplayType_CardDisplayType_AfterEvolution,
		})
	})
	evolved.DisplayType = penum.CardDisplayType_CardDisplayType_AfterEvolution
	if changed := evolvedDisplay.GetCommonResponse().GetUpdatedData().GetCards(); len(changed) != 1 || !proto.Equal(changed[0], evolved) {
		t.Fatalf("evolution display selection changed card training or returned incorrect display: %v", evolvedDisplay)
	}
	card.DisplayType = penum.CardDisplayType_CardDisplayType_AfterLimitBreak
	support := assigned.GetCommonResponse().GetUpdatedData().GetCardSupports()[0]
	support.RemovableTime = time.Now().Add(-time.Minute).UnixMilli()
	if err := opuser.UpsertUserCardSupport(ctx, testConfig.TestUserSuid, support); err != nil {
		t.Fatal(err)
	}
	removed := cardSupportCall(t, func(ctx context.Context) (*papi.CardRemoveSupportResponse, error) {
		return client.RemoveSupport(ctx, &papi.CardRemoveSupportRequest{Number: 1})
	})
	removedUpdate := removed.GetCommonResponse().GetUpdatedData()
	if len(removedUpdate.GetCards()) != 1 || !proto.Equal(removedUpdate.Cards[0], card) {
		t.Fatalf("RemoveSupport did not restore base card: got %v, want %v", removedUpdate.GetCards(), card)
	}
	if len(removedUpdate.GetCardSupports()) != 1 || removedUpdate.CardSupports[0].CardId != "" || removedUpdate.CardSupports[0].RemovableTime != support.RemovableTime {
		t.Fatalf("RemoveSupport did not clear the support slot: %v", removedUpdate.GetCardSupports())
	}
	if userItemAmount(t, "item-card_skill") != beforeSkill || userItemAmount(t, "item-card_ability") != beforeAbility {
		t.Fatal("RemoveSupport did not refund exactly the materials spent on support training")
	}
	lockedStory, err := opuser.FindUserStory(ctx, testConfig.TestUserSuid, readStory.StoryId)
	if err != nil || !lockedStory.GetIsInvalid() || lockedStory.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read {
		t.Fatalf("RemoveSupport lost story read progress or did not relock it: %v, %v", lockedStory, err)
	}
	if len(removed.GetCommonResponse().GetDeletedData().GetStories()) == 0 {
		t.Fatal("RemoveSupport omitted relocked stories from deletedData")
	}
	cardSupportExpectCode(t, codes.FailedPrecondition, func(ctx context.Context) error {
		_, err := client.RemoveSupport(ctx, &papi.CardRemoveSupportRequest{Number: 1})
		return err
	})
	// Reuse the emptied slot, then use a second slot. Multiple removals must not
	// collide on the unique per-user supported-card index.
	for _, number := range []int32{1, 2} {
		reassigned := cardSupportCall(t, func(ctx context.Context) (*papi.CardSetSupportResponse, error) {
			return client.SetSupport(ctx, &papi.CardSetSupportRequest{Number: number, CardId: card.CardId})
		})
		restoredStory, err := opuser.FindUserStory(ctx, testConfig.TestUserSuid, readStory.StoryId)
		if err != nil || restoredStory.GetIsInvalid() || restoredStory.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read {
			t.Fatalf("SetSupport did not restore story read state: %v, %v", restoredStory, err)
		}
		reusedSupport := reassigned.GetCommonResponse().GetUpdatedData().GetCardSupports()[0]
		reusedSupport.RemovableTime = time.Now().Add(-time.Minute).UnixMilli()
		if err := opuser.UpsertUserCardSupport(ctx, testConfig.TestUserSuid, reusedSupport); err != nil {
			t.Fatal(err)
		}
		repeated := cardSupportCall(t, func(ctx context.Context) (*papi.CardRemoveSupportResponse, error) {
			return client.RemoveSupport(ctx, &papi.CardRemoveSupportRequest{Number: number})
		})
		if len(repeated.GetRewards()) != 0 {
			t.Fatalf("support reassignment duplicated training refunds: %v", repeated.GetRewards())
		}
	}
	got := cardSupportCall(t, func(ctx context.Context) (*papi.UserGetResponse, error) {
		return papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	})
	found := false
	for _, stored := range got.GetCards() {
		if stored.CardId == card.CardId {
			found = true
			if !proto.Equal(stored, card) {
				t.Fatalf("User.Get returned incorrect card after support removal: %v", stored)
			}
		}
	}
	if !found || len(got.GetCardSupports()) != 0 {
		t.Fatalf("User.Get omitted card or retained occupied support slots: found=%v supports=%v", found, got.GetCardSupports())
	}
}

func cardSupportScenarioMaster(t *testing.T, ctx context.Context) *pmaster.Card {
	t.Helper()
	masters, err := opmaster.FindAllCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, master := range masters {
		if master.InitialRarity != 5 || master.ActivityAbilityId == "" || master.SkillId1 == "" || len(master.Stories) < 3 {
			continue
		}
		images, err := opmaster.FindCardImages(ctx, master.Id)
		if err != nil {
			t.Fatal(err)
		}
		hasAfter := false
		for _, image := range images {
			hasAfter = hasAfter || image.DisplayType == penum.CardDisplayType_CardDisplayType_AfterLimitBreak
		}
		if hasAfter {
			return master
		}
	}
	t.Fatal("no master card can exercise support, stories, and display changes")
	return nil
}

func cardSupportCall[T any](t *testing.T, call func(context.Context) (T, error)) T {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	response, err := call(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func cardSupportExpectCode(t *testing.T, want codes.Code, call func(context.Context) error) {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if err := call(ctx); status.Code(err) != want {
		t.Fatalf("request returned %v, want %v", err, want)
	}
}
