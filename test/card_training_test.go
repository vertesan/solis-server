package test

import (
	"context"
	"errors"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestCardTrainingScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	ctx := context.Background()
	suid := testConfig.TestUserSuid
	client := papi.NewCardClient(testConfig.GrpcConn)
	master, ability, nextAbility, initial, target := prepareCardTrainingScenario(t)
	cardID := initial.CardId
	skill, err := opmaster.FindSkill(ctx, master.SkillId1)
	if err != nil {
		t.Fatal(err)
	}
	var nextSkill *pmaster.SkillLevel
	for _, tier := range skill.Levels {
		if tier.Level == 2 && tier.RequiredCardLevel <= target.Level {
			nextSkill = tier
		}
	}
	if nextSkill == nil {
		t.Fatal("selected card has no trainable first skill")
	}
	rank, err := opmaster.FindCardRank(ctx, initial.Rank)
	if err != nil {
		t.Fatal(err)
	}
	initialRank, err := opmaster.FindCardRank(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	rankCost := int64(rank.RequiredExp - initialRank.RequiredExp)
	initialAmounts := map[string]int64{
		"item-card_enhance": target.RequiredExp + 100,
		"item-card_skill":   int64(nextSkill.RequiredItemAmount)*3 + 100,
		"item-card_ability": int64(nextAbility.RequiredItemAmount)*3 + 100,
		"item-card_rank":    rankCost + 100,
	}
	for itemID, amount := range initialAmounts {
		setCardTrainingItem(t, itemID, amount)
	}
	// Rank enhancement is a separate, unimplemented API. The fixture includes
	// its paid progress so Enhance must apply its cap extension and Reset must
	// return the rank materials together with the large level EXP investment.
	setCardTrainingItem(t, "item-card_rank", initialAmounts["item-card_rank"]-rankCost)

	// An unowned or still-locked ability must never consume materials.
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ActivityAbilityEnhance(grpcCtx, &papi.CardActivityAbilityEnhanceRequest{CardId: cardID, AbilityId: ability.Id})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("locked activity ability returned %v", err)
	}
	if got := userItemAmount(t, "item-card_ability"); got != initialAmounts["item-card_ability"] {
		t.Fatalf("rejected activity ability changed materials: %d", got)
	}

	enhanced := cardTrainingCall(t, func(ctx context.Context) (*papi.CardEnhanceResponse, error) {
		return client.Enhance(ctx, &papi.CardEnhanceRequest{CardId: cardID, Level: target.Level})
	})
	if got := enhanced.GetCommonResponse().GetUpdatedData().GetCards(); len(got) != 1 || got[0].ActivityAbilityId != ability.Id {
		t.Fatalf("card enhancement did not unlock the activity ability: %v", enhanced)
	}
	setCardTrainingItem(t, "item-card_ability", 0)
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ActivityAbilityEnhance(grpcCtx, &papi.CardActivityAbilityEnhanceRequest{CardId: cardID, AbilityId: ability.Id})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("activity enhancement with insufficient materials returned %v", err)
	}
	afterFailure, err := opuser.FindUserCard(ctx, suid, cardID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.ActivityAbilityLevel != 1 || afterFailure.BaseActivityAbilityLevel != 1 {
		t.Fatalf("rejected enhancement changed card training: %v", afterFailure)
	}
	setCardTrainingItem(t, "item-card_ability", initialAmounts["item-card_ability"])

	trained := cardTrainingCall(t, func(ctx context.Context) (*papi.CardActivityAbilityEnhanceResponse, error) {
		return client.ActivityAbilityEnhance(ctx, &papi.CardActivityAbilityEnhanceRequest{CardId: cardID, AbilityId: ability.Id})
	})
	trainedCards := trained.GetCommonResponse().GetUpdatedData().GetCards()
	if len(trainedCards) != 1 || trainedCards[0].ActivityAbilityLevel != nextAbility.Level || trainedCards[0].BaseActivityAbilityLevel != nextAbility.Level {
		t.Fatalf("activity enhancement did not persist base and effective levels: %v", trained)
	}
	if got := userItemAmount(t, "item-card_ability"); got != initialAmounts["item-card_ability"]-int64(nextAbility.RequiredItemAmount) {
		t.Fatalf("activity enhancement material balance = %d", got)
	}
	cardTrainingCall(t, func(ctx context.Context) (*papi.CardSkillEnhanceResponse, error) {
		return client.SkillEnhance(ctx, &papi.CardSkillEnhanceRequest{CardId: cardID, SkillId: master.SkillId1})
	})
	trainedCard, err := opuser.FindUserCard(ctx, suid, cardID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range master.Stories {
		if err := opuser.UpsertUserStory(ctx, suid, &ptransaction.UserStory{
			StoryId: entry.StoryId, StatusType: penum.StoryStatusType_StoryStatusType_Read,
		}); err != nil {
			t.Fatal(err)
		}
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	if setting.CardResetStoneAmount > 0 {
		if err := opuser.UpsertUserBalance(ctx, suid, &ptransaction.UserBalance{}); err != nil {
			t.Fatal(err)
		}
		grpcCtx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Reset(grpcCtx, &papi.CardResetRequest{CardId: cardID})
		cancel()
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("card reset without dia returned %v", err)
		}
		afterFailure, err = opuser.FindUserCard(ctx, suid, cardID)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(trainedCard, afterFailure) {
			t.Fatal("rejected reset changed card training")
		}
		if got := userItemAmount(t, "item-card_enhance"); got != initialAmounts["item-card_enhance"]-trainedCard.TotalExp {
			t.Fatalf("rejected reset refunded EXP: %d", got)
		}
	}
	freeBefore, paidBefore := setting.CardResetStoneAmount/2, setting.CardResetStoneAmount+20
	if err := opuser.UpsertUserBalance(ctx, suid, &ptransaction.UserBalance{FreeBalance: freeBefore, PaidBalance: paidBefore}); err != nil {
		t.Fatal(err)
	}
	reset := cardTrainingCall(t, func(ctx context.Context) (*papi.CardResetResponse, error) {
		return client.Reset(ctx, &papi.CardResetRequest{CardId: cardID})
	})
	resetCards := reset.GetCommonResponse().GetUpdatedData().GetCards()
	if len(resetCards) != 1 {
		t.Fatalf("reset card post-image missing: %v", reset)
	}
	resetCard := resetCards[0]
	if resetCard.Level != 1 || resetCard.BaseLevel != 1 || resetCard.TotalExp != 0 || resetCard.Rank != 0 || resetCard.RankTotalExp != initialRank.RequiredExp ||
		resetCard.SkillLevel1 != 1 || resetCard.BaseSkillLevel1 != 1 || resetCard.ActivityAbilityLevel != 1 || resetCard.BaseActivityAbilityLevel != 1 {
		t.Fatalf("reset retained training: %v", resetCard)
	}
	if resetCard.Rarity != trainedCard.Rarity || resetCard.RarityTotalExp != trainedCard.RarityTotalExp || resetCard.EvolutionLevel != trainedCard.EvolutionLevel ||
		resetCard.EvolutionLevelTotalExp != trainedCard.EvolutionLevelTotalExp || resetCard.ObtainedTime != trainedCard.ObtainedTime || resetCard.LastEnhanceTime != trainedCard.LastEnhanceTime {
		t.Fatalf("reset changed permanent card progress: %v", resetCard)
	}
	if resetCard.Vocal != initial.Vocal || resetCard.Dance != initial.Dance || resetCard.Visual != initial.Visual || resetCard.Stamina != initial.Stamina ||
		resetCard.PhotoEquipableCount != initial.PhotoEquipableCount || resetCard.ActivityAbilityId != initial.ActivityAbilityId {
		t.Fatalf("reset did not restore initial attributes/unlocks: %v", resetCard)
	}
	expectedRefunds := map[string]int64{
		"item-card_enhance": trainedCard.TotalExp,
		"item-card_skill":   int64(nextSkill.RequiredItemAmount),
		"item-card_ability": int64(nextAbility.RequiredItemAmount),
		"item-card_rank":    rankCost,
	}
	if len(reset.Rewards) != len(expectedRefunds) {
		t.Fatalf("unexpected reset refunds: %v", reset.Rewards)
	}
	for _, reward := range reset.Rewards {
		if reward.ResourceType != penum.ResourceType_ResourceType_Item || expectedRefunds[reward.ResourceId] != reward.Amount {
			t.Fatalf("incorrect reset refund: %v", reward)
		}
	}
	for itemID, initialAmount := range initialAmounts {
		if got := userItemAmount(t, itemID); got != initialAmount {
			t.Fatalf("reset did not fully refund %s: got %d, want %d", itemID, got, initialAmount)
		}
	}
	balance, err := opuser.FindUserBalance(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	if balance.FreeBalance != 0 || balance.PaidBalance != paidBefore-(setting.CardResetStoneAmount-freeBefore) {
		t.Fatalf("reset did not spend free dia before paid dia: %v", balance)
	}
	locked := reset.GetCommonResponse().GetDeletedData().GetStories()
	if len(locked) < 1 {
		t.Fatalf("reset omitted re-locked stories: %v", reset)
	}
	for _, story := range locked {
		stored, err := opuser.FindUserStory(ctx, suid, story.StoryId)
		if err != nil || !stored.GetIsInvalid() || stored.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read {
			t.Fatalf("reset lost saved story progress: %v, %v", stored, err)
		}
	}
	userGet := cardTrainingCall(t, func(ctx context.Context) (*papi.UserGetResponse, error) {
		return papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	})
	var fetched *ptransaction.UserCard
	for _, card := range userGet.Cards {
		if card.CardId == cardID {
			fetched = card
		}
	}
	if !proto.Equal(resetCard, fetched) {
		t.Fatalf("User.Get lost reset state: %v", fetched)
	}
	rarity, err := opmaster.FindCardRarity(ctx, int64(resetCard.Rarity))
	if err != nil {
		t.Fatal(err)
	}
	if target.Level > rarity.LevelLimit {
		grpcCtx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Enhance(grpcCtx, &papi.CardEnhanceRequest{CardId: cardID, Level: target.Level})
		cancel()
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("reset card retained rank level-cap extension: %v", err)
		}
		if got := userItemAmount(t, "item-card_enhance"); got != initialAmounts["item-card_enhance"] {
			t.Fatalf("rejected rankless enhancement changed EXP materials: %d", got)
		}
	}

	// Re-enhancing restores already-read stories without replaying their rewards.
	reenhanced := cardTrainingCall(t, func(ctx context.Context) (*papi.CardEnhanceResponse, error) {
		return client.Enhance(ctx, &papi.CardEnhanceRequest{CardId: cardID, Level: min(target.Level, rarity.LevelLimit)})
	})
	reactivated := reenhanced.GetCommonResponse().GetUpdatedData().GetStories()
	if len(reactivated) != len(locked) {
		t.Fatalf("re-enhancement omitted reactivated stories: got %v, locked %v", reactivated, locked)
	}
	for _, story := range reactivated {
		stored, err := opuser.FindUserStory(ctx, suid, story.StoryId)
		if err != nil || stored.GetIsInvalid() || stored.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read {
			t.Fatalf("re-enhancement failed to retain read story: %v, %v", stored, err)
		}
	}
}

func prepareCardTrainingScenario(t *testing.T) (*pmaster.Card, *pmaster.ActivityAbility, *pmaster.ActivityAbilityLevel, *ptransaction.UserCard, *pmaster.CardLevel) {
	t.Helper()
	ctx := context.Background()
	var rarity *pmaster.CardRarity
	for number := int64(1); ; number++ {
		entry, err := opmaster.FindCardRarity(ctx, number)
		if errors.Is(err, mongo.ErrNoDocuments) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rarity = entry
	}
	if rarity == nil {
		t.Fatal("master has no card rarities")
	}
	var rank *pmaster.CardRank
	for number := int32(0); ; number++ {
		entry, err := opmaster.FindCardRank(ctx, number)
		if errors.Is(err, mongo.ErrNoDocuments) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rank = entry
	}
	if rank == nil || rank.Rank == 0 {
		t.Fatal("master has no card rank extensions")
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := opmaster.FindManagerLevel(ctx, user.ManagerLevel)
	if err != nil {
		t.Fatal(err)
	}
	target, err := opmaster.FindCardLevel(ctx, min(rarity.LevelLimit+rank.LevelLimitUpAmount, manager.MaxCardLevel))
	if err != nil {
		t.Fatal(err)
	}
	masters, err := opmaster.FindAllCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, master := range masters {
		if master.ActivityAbilityId == "" || master.SkillId1 == "" || len(master.Stories) < 3 || master.CardEvolutionLevelGroupId != "" {
			continue
		}
		ability, err := opmaster.FindActivityAbility(ctx, master.ActivityAbilityId)
		if err != nil {
			t.Fatal(err)
		}
		var next *pmaster.ActivityAbilityLevel
		for _, tier := range ability.Levels {
			if tier.Level == 2 && tier.RequiredCardLevel <= target.Level && tier.RequiredItemAmount > 0 {
				next = tier
			}
		}
		if next == nil {
			continue
		}
		card, err := usercard.SpawnUserCard(ctx, master)
		if err != nil {
			t.Fatal(err)
		}
		if card.ActivityAbilityId != "" {
			continue
		}
		attributes, err := usercard.CalcUserCardBaseAttr(ctx, master, 1, rarity.Rarity)
		if err != nil {
			t.Fatal(err)
		}
		card.Rarity, card.RarityTotalExp = rarity.Rarity, rarity.RequiredExp
		card.Rank, card.RankTotalExp = rank.Rank, rank.RequiredExp
		card.Vocal, card.BaseVocal = attributes.Vocal, attributes.Vocal
		card.Dance, card.BaseDance = attributes.Dance, attributes.Dance
		card.Visual, card.BaseVisual = attributes.Visual, attributes.Visual
		card.Stamina, card.BaseStamina = attributes.Stamina, attributes.Stamina
		if err := opuser.UpsertUserCard(ctx, testConfig.TestUserSuid, card); err != nil {
			t.Fatal(err)
		}
		return master, ability, next, card, target
	}
	t.Fatal("no card can exercise activity training and story relocking")
	return nil, nil, nil, nil, nil
}

func setCardTrainingItem(t *testing.T, itemID string, amount int64) {
	t.Helper()
	if err := opuser.UpsertUserItem(context.Background(), testConfig.TestUserSuid, &ptransaction.UserItem{ItemId: itemID, Amount: amount}); err != nil {
		t.Fatal(err)
	}
}

func cardTrainingCall[T any](t *testing.T, call func(context.Context) (T, error)) T {
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
