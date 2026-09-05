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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

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
