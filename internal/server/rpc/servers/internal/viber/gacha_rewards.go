package viber

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	usercharacter "github.com/vertesan/solis-server/internal/makino/user/character"
	usercostume "github.com/vertesan/solis-server/internal/makino/user/costume"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	gachaCardLimitBreakItemID      = "item-card_limit_break"
	gachaCardLimitBreakGiftMessage = "才能開花上限による報酬です"
)

type GachaOverflowGiftBatch struct {
	PostedTime int64
	ByCardID   map[string]int64
}

type gachaDuplicateCardProgress struct {
	additionalGifts              []*ptransaction.Reward
	evolutionRewardResults       []*ptransaction.RewardResult
	increasedAfterEvolutionLevel int32
}

func (b *GachaOverflowGiftBatch) add(cardID string, amount int64) error {
	if b == nil || cardID == "" || amount <= 0 {
		return status.Error(codes.Internal, "gacha overflow gift batch entry is invalid")
	}
	if b.ByCardID[cardID] > math.MaxInt64-amount {
		return status.Error(codes.FailedPrecondition, "gacha overflow gift amount overflows")
	}
	b.ByCardID[cardID] += amount
	return nil
}

func GrantGachaReward(
	ctx context.Context,
	suid string,
	reward *pmaster.RewardResource,
	overflowGifts *GachaOverflowGiftBatch,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if reward == nil || reward.GetAmount() <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "gacha reward must have a positive amount")
	}
	if reward.GetResourceType() != penum.ResourceType_ResourceType_Card {
		return GrantExchangeRewards(ctx, suid, reward, updated)
	}
	if reward.GetAmount() != 1 {
		return nil, status.Error(codes.FailedPrecondition, "gacha card rewards must have amount one")
	}
	return grantGachaCard(ctx, suid, reward.GetResourceId(), overflowGifts, updated)
}

func grantGachaCard(
	ctx context.Context,
	suid string,
	cardID string,
	overflowGifts *GachaOverflowGiftBatch,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	masterCard, err := opmaster.FindCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	card, err := opuser.FindUserCard(ctx, suid, cardID)
	if err == nil {
		// Duplicate compensation is determined from the card state at the start
		// of this occurrence. The duplicate EXP applied below may cross a rarity
		// threshold, but that crossing must not retroactively increase this
		// copy's compensation.
		duplicateRewardRarity := card.GetRarity()
		progress, err := grantGachaDuplicateCardExp(ctx, suid, masterCard, card, overflowGifts, updated)
		if err != nil {
			return nil, err
		}
		duplicate, err := opmaster.FindCardDuplicateReward(ctx, masterCard.GetInitialRarity(), duplicateRewardRarity)
		if err != nil {
			return nil, err
		}
		resources, err := ParseAutoReward(duplicate.GetRewardId())
		if err != nil {
			return nil, err
		}
		if resources == nil {
			masterReward, err := opmaster.FindReward(ctx, duplicate.GetRewardId())
			if err != nil {
				return nil, err
			}
			resources = masterReward.GetResources()
		}
		primary := gachaRewardResult(penum.ResourceType_ResourceType_Card, cardID, 1, 1, 1)
		primary.AdditionalGifts = progress.additionalGifts
		primary.IncreasedAfterEvolutionLevel = progress.increasedAfterEvolutionLevel
		results := []*ptransaction.RewardResult{primary}
		results = append(results, progress.evolutionRewardResults...)
		for _, resource := range resources {
			granted, err := GrantGachaReward(ctx, suid, resource, overflowGifts, updated)
			if err != nil {
				return nil, err
			}
			primary.DuplicateRewardResults = append(primary.DuplicateRewardResults, granted...)
			results = append(results, granted...)
		}
		return results, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}

	card, err = usercard.SpawnUserCard(ctx, masterCard)
	if err != nil {
		return nil, err
	}
	if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
		return nil, err
	}
	updated.Cards = append(updated.Cards, card)
	missions, err := AdvanceUserMissions(
		ctx,
		suid,
		penum.MissionType_MissionType_CharacterTotalCardRarityAchievement,
		int64(card.GetRarity()),
		card.GetCardId(),
		masterCard.GetCharacterId(),
	)
	if err != nil {
		return nil, err
	}
	updated.Missions = append(updated.Missions, missions...)
	primary := gachaRewardResult(penum.ResourceType_ResourceType_Card, cardID, 0, 1, 1)
	results := []*ptransaction.RewardResult{primary}
	toyResults, err := grantGachaCardShowcaseToys(ctx, suid, cardID, updated)
	if err != nil {
		return nil, err
	}
	primary.AdditionalGifts = append(primary.AdditionalGifts, toyResults...)

	characterResults, err := ensureGachaCharacter(ctx, suid, masterCard.GetCharacterId(), updated)
	if err != nil {
		return nil, err
	}
	results = append(results, characterResults...)
	if masterCard.GetRewardCostumeId() != "" {
		costumeResults, err := ensureGachaCostume(ctx, suid, masterCard.GetRewardCostumeId(), false, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, costumeResults...)
	}
	hairs := append([]string(nil), masterCard.GetRewardHairIds()...)
	if masterCard.GetRewardHairId() != "" {
		hairs = append(hairs, masterCard.GetRewardHairId())
	}
	seenHair := make(map[string]struct{}, len(hairs))
	for _, hairID := range hairs {
		if hairID == "" {
			continue
		}
		if _, ok := seenHair[hairID]; ok {
			continue
		}
		seenHair[hairID] = struct{}{}
		hairResult, err := ensureGachaHair(ctx, suid, hairID, updated)
		if err != nil {
			return nil, err
		}
		if hairResult != nil {
			results = append(results, hairResult)
		}
	}
	storyResults, err := ensureGachaCardStories(ctx, suid, masterCard, updated)
	if err != nil {
		return nil, err
	}
	results = append(results, storyResults...)
	return results, nil
}

func grantGachaDuplicateCardExp(
	ctx context.Context,
	suid string,
	masterCard *pmaster.Card,
	card *ptransaction.UserCard,
	overflowGifts *GachaOverflowGiftBatch,
	updated *papi.UpdatedData,
) (*gachaDuplicateCardProgress, error) {
	progress := &gachaDuplicateCardProgress{}
	duplicateExp, err := opmaster.FindCardDuplicateExp(ctx, masterCard.GetInitialRarity())
	if err != nil {
		return nil, err
	}
	if duplicateExp.GetExp() <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "card duplicate EXP must be positive")
	}
	rarities, err := opmaster.FindAllCardRarities(ctx)
	if err != nil {
		return nil, err
	}
	if len(rarities) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "card rarity master data is empty")
	}
	maxRarity := rarities[len(rarities)-1]
	maxExp := int64(maxRarity.GetRequiredExp())
	currentExp := int64(card.GetRarityTotalExp())
	if maxExp <= 0 || currentExp < 0 || currentExp > maxExp {
		return nil, status.Error(codes.FailedPrecondition, "card rarity EXP is inconsistent with master data")
	}

	previousRarity := card.GetRarity()
	previousEvolutionLevel := card.GetEvolutionLevel()
	gainedExp := int64(duplicateExp.GetExp())
	appliedExp := min(gainedExp, maxExp-currentExp)
	overflowExp := gainedExp - appliedExp
	card.RarityTotalExp = int32(currentExp + appliedExp)
	for _, rarity := range rarities {
		if rarity.GetRequiredExp() <= card.GetRarityTotalExp() && rarity.GetRarity() > card.GetRarity() {
			card.Rarity = rarity.GetRarity()
		}
	}
	progress.increasedAfterEvolutionLevel, err = usercard.ApplyDuplicateEvolution(ctx, masterCard, card)
	if err != nil {
		return nil, err
	}
	if card.GetRarity() > previousRarity {
		baseAttributes, err := usercard.CalcUserCardBaseAttr(ctx, masterCard, card.GetBaseLevel(), card.GetRarity())
		if err != nil {
			return nil, err
		}
		card.BaseVocal, card.BaseDance = baseAttributes.Vocal, baseAttributes.Dance
		card.BaseVisual, card.BaseStamina = baseAttributes.Visual, baseAttributes.Stamina
		effectiveAttributes := baseAttributes
		if card.GetSupported() {
			effectiveAttributes, err = usercard.CalcUserCardBaseAttr(ctx, masterCard, card.GetLevel(), card.GetRarity())
			if err != nil {
				return nil, err
			}
		}
		card.Vocal, card.Dance = effectiveAttributes.Vocal, effectiveAttributes.Dance
		card.Visual, card.Stamina = effectiveAttributes.Visual, effectiveAttributes.Stamina
	}
	if card.GetEvolutionLevel() > previousEvolutionLevel {
		if err := applyGachaCardEvolutionUnlocks(ctx, masterCard, card); err != nil {
			return nil, err
		}
	}
	if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
		return nil, err
	}
	putUpdatedRewardCard(updated, card)
	rarityIncrease := card.GetRarity() - previousRarity
	if rarityIncrease > 0 {
		for _, missionType := range []penum.MissionType{
			penum.MissionType_MissionType_CardLimitBreakCount,
			penum.MissionType_MissionType_CharacterLimitBreakCount,
			penum.MissionType_MissionType_CharacterTotalCardRarityAchievement,
		} {
			missions, err := AdvanceUserMissions(
				ctx,
				suid,
				missionType,
				int64(rarityIncrease),
				card.GetCardId(),
				masterCard.GetCharacterId(),
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = append(updated.Missions, missions...)
		}
	}
	if card.GetEvolutionLevel() > previousEvolutionLevel {
		progress.evolutionRewardResults, err = grantGachaEvolutionRewards(
			ctx, suid, masterCard, previousEvolutionLevel, card.GetEvolutionLevel(), updated,
		)
		if err != nil {
			return nil, err
		}
		storyResults, err := ensureGachaCardStoriesAtProgress(
			ctx, suid, masterCard, card.GetLevel(), card.GetEvolutionLevel(), updated,
		)
		if err != nil {
			return nil, err
		}
		progress.evolutionRewardResults = append(progress.evolutionRewardResults, storyResults...)
	}
	if overflowExp == 0 {
		return progress, nil
	}
	if _, err := opmaster.FindItem(ctx, gachaCardLimitBreakItemID); err != nil {
		return nil, err
	}
	if overflowGifts != nil {
		if err := overflowGifts.add(card.GetCardId(), overflowExp); err != nil {
			return nil, err
		}
	} else {
		if err := PersistGachaCardLimitBreakGift(ctx, suid, overflowExp, time.Now().UnixMilli()); err != nil {
			return nil, err
		}
		if err := EnsureGachaGiftNotification(ctx, suid, updated); err != nil {
			return nil, err
		}
	}
	progress.additionalGifts = []*ptransaction.Reward{{
		ResourceType: penum.ResourceType_ResourceType_Item,
		ResourceId:   gachaCardLimitBreakItemID,
		Amount:       overflowExp,
	}}
	return progress, nil
}

func applyGachaCardEvolutionUnlocks(
	ctx context.Context,
	masterCard *pmaster.Card,
	card *ptransaction.UserCard,
) error {
	basePhoto, baseSkills, baseAbilities, err := usercard.CalcUnlocksWithEvolution(
		ctx, card.GetBaseLevel(), card.GetEvolutionLevel(), masterCard,
	)
	if err != nil {
		return err
	}
	card.BasePhotoEquipableCount = basePhoto
	card.BaseSkillId1, card.BaseSkillId2 = baseSkills[0], baseSkills[1]
	card.BaseSkillId3, card.BaseSkillId4 = baseSkills[2], baseSkills[3]
	card.BaseLiveAbilityId, card.BaseActivityAbilityId = baseAbilities[0], baseAbilities[1]

	photo, skills, abilities := basePhoto, baseSkills, baseAbilities
	if card.GetSupported() {
		photo, skills, abilities, err = usercard.CalcUnlocksWithEvolution(
			ctx, card.GetLevel(), card.GetEvolutionLevel(), masterCard,
		)
		if err != nil {
			return err
		}
	}
	card.PhotoEquipableCount = photo
	card.SkillId1, card.SkillId2 = skills[0], skills[1]
	card.SkillId3, card.SkillId4 = skills[2], skills[3]
	card.LiveAbilityId, card.ActivityAbilityId = abilities[0], abilities[1]
	return nil
}

func grantGachaEvolutionRewards(
	ctx context.Context,
	suid string,
	masterCard *pmaster.Card,
	previousLevel int32,
	afterLevel int32,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	evolutions, err := opmaster.FindCardEvolutions(ctx, masterCard.GetId(), previousLevel, afterLevel)
	if err != nil {
		return nil, err
	}
	results := make([]*ptransaction.RewardResult, 0, len(evolutions)*2)
	for _, evolution := range evolutions {
		if evolution.GetRewardCostumeId() != "" {
			granted, err := ensureGachaCostume(ctx, suid, evolution.GetRewardCostumeId(), false, updated)
			if err != nil {
				return nil, err
			}
			results = append(results, granted...)
		}
		if evolution.GetRewardHairId() != "" {
			granted, err := ensureGachaHair(ctx, suid, evolution.GetRewardHairId(), updated)
			if err != nil {
				return nil, err
			}
			if granted != nil {
				results = append(results, granted)
			}
		}
	}
	return results, nil
}

func PersistGachaCardLimitBreakGift(ctx context.Context, suid string, amount, postedTime int64) error {
	return opuser.UpsertUserGift(ctx, suid, &ptransaction.UserGift{
		GiftId:       uuid.NewString(),
		ResourceType: penum.ResourceType_ResourceType_Item,
		ResourceId:   gachaCardLimitBreakItemID,
		Amount:       amount,
		Message:      gachaCardLimitBreakGiftMessage,
		PostedTime:   postedTime,
	})
}

func ensureGachaCharacter(
	ctx context.Context,
	suid string,
	characterID string,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if characterID == "" {
		return nil, nil
	}
	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return nil, err
	}
	for _, character := range characters {
		if character.GetCharacterId() == characterID {
			return nil, nil
		}
	}
	masterCharacter, err := opmaster.FindCharacter(ctx, characterID)
	if err != nil {
		return nil, err
	}
	character, err := usercharacter.SpawnUserCharacter(ctx, masterCharacter)
	if err != nil {
		return nil, err
	}
	if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
		return nil, err
	}
	updated.Characters = append(updated.Characters, character)
	results := []*ptransaction.RewardResult{
		gachaRewardResult(penum.ResourceType_ResourceType_Character, characterID, 0, 1, 1),
	}
	seenCostumes := make(map[string]struct{}, 2)
	for _, costumeID := range []string{masterCharacter.GetDefaultCostumeId(), masterCharacter.GetDefaultLiveCostumeId()} {
		if costumeID == "" {
			continue
		}
		if _, ok := seenCostumes[costumeID]; ok {
			continue
		}
		seenCostumes[costumeID] = struct{}{}
		costumeResults, err := ensureGachaCostume(ctx, suid, costumeID, true, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, costumeResults...)
	}
	return results, nil
}

func ensureGachaCostume(
	ctx context.Context,
	suid string,
	costumeID string,
	checked bool,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	_, err := opuser.FindUserCostume(ctx, suid, costumeID)
	if err == nil {
		return nil, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	costume, err := usercostume.SpawnUserCostumeById(ctx, costumeID)
	if err != nil {
		return nil, err
	}
	costume.Checked = checked
	if err := opuser.UpsertUserCostume(ctx, suid, costume); err != nil {
		return nil, err
	}
	updated.Costumes = append(updated.Costumes, costume)
	results := []*ptransaction.RewardResult{
		gachaRewardResult(penum.ResourceType_ResourceType_Costume, costumeID, 0, 1, 1),
	}
	masterCostume, err := opmaster.FindCostume(ctx, costumeID)
	if err != nil {
		return nil, err
	}
	if masterCostume.GetDefaultHairId() != "" {
		hairResult, err := ensureGachaHair(ctx, suid, masterCostume.GetDefaultHairId(), updated)
		if err != nil {
			return nil, err
		}
		if hairResult != nil {
			results = append(results, hairResult)
		}
	}
	return results, nil
}

func ensureGachaHair(
	ctx context.Context,
	suid string,
	hairID string,
	updated *papi.UpdatedData,
) (*ptransaction.RewardResult, error) {
	hairs, err := opuser.FindAllUserHairs(ctx, suid)
	if err != nil {
		return nil, err
	}
	for _, hair := range hairs {
		if hair.GetHairId() == hairID {
			return nil, nil
		}
	}
	// A successful master lookup protects user data from catalog typos.
	if _, err := opmaster.FindHair(ctx, hairID); err != nil {
		return nil, err
	}
	hair := &ptransaction.UserHair{HairId: hairID, Checked: true}
	if err := opuser.UpsertUserHair(ctx, suid, hair); err != nil {
		return nil, err
	}
	updated.Hairs = append(updated.Hairs, hair)
	return gachaRewardResult(penum.ResourceType_ResourceType_Hair, hairID, 0, 1, 1), nil
}

func grantGachaCardShowcaseToys(
	ctx context.Context,
	suid string,
	cardID string,
	updated *papi.UpdatedData,
) ([]*ptransaction.Reward, error) {
	toys, err := opmaster.FindAllShowcaseToys(ctx)
	if err != nil {
		return nil, err
	}
	rewards := make([]*ptransaction.Reward, 0, 2)
	for _, toy := range toys {
		if !strings.HasSuffix(toy.GetId(), "_"+cardID) {
			continue
		}
		resource := &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_ShowcaseToy,
			ResourceId:   toy.GetId(),
			Amount:       1,
		}
		gift := &ptransaction.UserGift{
			GiftId:       uuid.NewString(),
			ResourceType: resource.GetResourceType(),
			ResourceId:   resource.GetResourceId(),
			Amount:       1,
			PostedTime:   time.Now().UnixMilli(),
		}
		if err := opuser.UpsertUserGift(ctx, suid, gift); err != nil {
			return nil, err
		}
		rewards = append(rewards, &ptransaction.Reward{
			ResourceType: resource.GetResourceType(),
			ResourceId:   resource.GetResourceId(),
			Amount:       1,
		})
	}
	if len(rewards) > 0 {
		if err := EnsureGachaGiftNotification(ctx, suid, updated); err != nil {
			return nil, err
		}
	}
	return rewards, nil
}

func EnsureGachaGiftNotification(ctx context.Context, suid string, updated *papi.UpdatedData) error {
	return SetGiftNotification(ctx, suid, true, updated)
}

func ensureGachaCardStories(
	ctx context.Context,
	suid string,
	masterCard *pmaster.Card,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	return ensureGachaCardStoriesAtProgress(ctx, suid, masterCard, 1, 0, updated)
}

func ensureGachaCardStoriesAtProgress(
	ctx context.Context,
	suid string,
	masterCard *pmaster.Card,
	level int32,
	evolutionLevel int32,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	releases, err := usercard.CalcReleaseCountsWithEvolution(ctx, level, evolutionLevel, masterCard)
	if err != nil {
		return nil, err
	}
	storyCount := releases[penum.CardLevelReleaseType_CardLevelReleaseType_Story]
	if storyCount <= 0 {
		return nil, nil
	}
	if storyCount > int32(len(masterCard.GetStories())) {
		storyCount = int32(len(masterCard.GetStories()))
	}
	existing, err := opuser.FindAllUserStories(ctx, suid)
	if err != nil {
		return nil, err
	}
	existingIDs := make(map[string]struct{}, len(existing))
	for _, story := range existing {
		existingIDs[story.GetStoryId()] = struct{}{}
	}
	stories := make([]*ptransaction.UserStory, 0, storyCount)
	results := make([]*ptransaction.RewardResult, 0, storyCount)
	for _, masterStory := range masterCard.GetStories()[:storyCount] {
		if _, ok := existingIDs[masterStory.GetStoryId()]; ok {
			continue
		}
		story := &ptransaction.UserStory{
			StoryId:    masterStory.GetStoryId(),
			StatusType: penum.StoryStatusType_StoryStatusType_Unread,
		}
		stories = append(stories, story)
		results = append(results, gachaRewardResult(
			penum.ResourceType_ResourceType_Story, story.GetStoryId(), 0, 1, 1,
		))
	}
	if err := opuser.UpsertManyUserStories(ctx, suid, stories); err != nil {
		return nil, err
	}
	updated.Stories = append(updated.Stories, stories...)
	return results, nil
}

func gachaRewardResult(resourceType penum.ResourceType, resourceID string, before, after, amount int64) *ptransaction.RewardResult {
	return &ptransaction.RewardResult{
		ResourceType: resourceType,
		ResourceId:   resourceID,
		BeforeAmount: before,
		AfterAmount:  after,
		AmountI64:    amount,
		IsNew:        before == 0 && after > 0,
	}
}
