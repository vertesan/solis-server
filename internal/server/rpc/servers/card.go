package servers

import (
	"context"
	"fmt"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type CardServer struct {
	papi.UnimplementedCardServer
}

const (
	cardEnhanceItemID    = "item-card_enhance"
	cardLimitBreakItemID = "item-card_limit_break"
	cardSkillItemID      = "item-card_skill"
	cardAbilityItemID    = "item-card_ability"
)

func (s *CardServer) Enhance(ctx context.Context, req *papi.CardEnhanceRequest) (*papi.CardEnhanceResponse, error) {
	if req.GetCardId() == "" || req.GetLevel() <= 0 {
		return nil, viber.InvalidArgumentf("cardId and a positive level are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardEnhanceResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		if card.Supported {
			return nil, viber.FailedPreconditionf("supported cards cannot be enhanced directly")
		}
		if req.Level <= card.BaseLevel {
			return nil, viber.FailedPreconditionf("target level must exceed the current base level")
		}
		levelIncrease := int64(req.Level - card.BaseLevel)
		cardRarity, err := opmaster.FindCardRarity(ctx, int64(card.Rarity))
		if err != nil {
			return nil, err
		}
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		managerLevel, err := opmaster.FindManagerLevel(ctx, user.ManagerLevel)
		if err != nil {
			return nil, err
		}
		if req.Level > cardRarity.LevelLimit || req.Level > managerLevel.MaxCardLevel {
			return nil, viber.FailedPreconditionf("target level exceeds the manager or rarity level limit")
		}
		target, err := opmaster.FindCardLevel(ctx, req.Level)
		if err != nil {
			return nil, err
		}
		cost := target.RequiredExp - card.TotalExp
		if cost <= 0 {
			return nil, fmt.Errorf("target level has no positive EXP delta")
		}
		item, err := viber.ConsumePermanentItem(ctx, suid, cardEnhanceItemID, cost)
		if err != nil {
			return nil, err
		}
		masterCard, err := opmaster.FindCard(ctx, card.CardId)
		if err != nil {
			return nil, err
		}
		attributes, err := usercard.CalcUserCardBaseAttr(ctx, masterCard, req.Level, card.Rarity)
		if err != nil {
			return nil, err
		}
		photoCount, skillIDs, abilityIDs, err := usercard.CalcUnlocks(ctx, req.Level, masterCard)
		if err != nil {
			return nil, err
		}
		stories, err := newlyUnlockedCardStories(ctx, suid, masterCard, req.Level)
		if err != nil {
			return nil, err
		}
		card.TotalExp = target.RequiredExp
		card.Level = req.Level
		card.BaseLevel = req.Level
		card.Vocal, card.Dance, card.Visual, card.Stamina = attributes.Vocal, attributes.Dance, attributes.Visual, attributes.Stamina
		card.BaseVocal, card.BaseDance, card.BaseVisual, card.BaseStamina = attributes.Vocal, attributes.Dance, attributes.Visual, attributes.Stamina
		card.PhotoEquipableCount, card.BasePhotoEquipableCount = photoCount, photoCount
		card.SkillId1, card.SkillId2, card.SkillId3 = skillIDs[0], skillIDs[1], skillIDs[2]
		card.BaseSkillId1, card.BaseSkillId2, card.BaseSkillId3 = skillIDs[0], skillIDs[1], skillIDs[2]
		card.LiveAbilityId, card.ActivityAbilityId = abilityIDs[0], abilityIDs[1]
		card.BaseLiveAbilityId, card.BaseActivityAbilityId = abilityIDs[0], abilityIDs[1]
		card.LastEnhanceTime = time.Now().UnixMilli()
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		if err := opuser.UpsertManyUserStories(ctx, suid, stories); err != nil {
			return nil, err
		}
		missions, err := viber.AdvanceUserMissions(
			ctx,
			suid,
			penum.MissionType_MissionType_CardLevelUpCount,
			levelIncrease,
			card.CardId,
			masterCard.CharacterId,
		)
		if err != nil {
			return nil, err
		}
		response = &papi.CardEnhanceResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Items:    []*ptransaction.UserItem{item},
			Cards:    []*ptransaction.UserCard{card},
			Stories:  stories,
			Missions: missions,
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *CardServer) LimitBreak(ctx context.Context, req *papi.CardLimitBreakRequest) (*papi.CardLimitBreakResponse, error) {
	if req.GetCardId() == "" || req.GetRarity() <= 0 {
		return nil, viber.InvalidArgumentf("cardId and a positive rarity are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardLimitBreakResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		if card.Supported {
			return nil, viber.FailedPreconditionf("supported cards cannot be enhanced directly")
		}
		if req.Rarity <= card.Rarity {
			return nil, viber.FailedPreconditionf("target rarity must exceed current rarity")
		}
		targetRarity, err := opmaster.FindCardRarity(ctx, int64(req.Rarity))
		if err != nil {
			return nil, err
		}
		cost := int64(targetRarity.RequiredExp - card.RarityTotalExp)
		if cost <= 0 {
			return nil, fmt.Errorf("target rarity has no positive EXP delta")
		}
		item, err := viber.ConsumePermanentItem(ctx, suid, cardLimitBreakItemID, cost)
		if err != nil {
			return nil, err
		}
		masterCard, err := opmaster.FindCard(ctx, card.CardId)
		if err != nil {
			return nil, err
		}
		attributes, err := usercard.CalcUserCardBaseAttr(ctx, masterCard, card.BaseLevel, req.Rarity)
		if err != nil {
			return nil, err
		}
		displayType, err := usercard.CalcCardDisplayType(ctx, card.CardId, req.Rarity, card.EvolutionLevel)
		if err != nil {
			return nil, err
		}
		card.Rarity = req.Rarity
		card.RarityTotalExp = targetRarity.RequiredExp
		card.DisplayType = displayType
		card.Vocal, card.Dance, card.Visual, card.Stamina = attributes.Vocal, attributes.Dance, attributes.Visual, attributes.Stamina
		card.BaseVocal, card.BaseDance, card.BaseVisual, card.BaseStamina = attributes.Vocal, attributes.Dance, attributes.Visual, attributes.Stamina
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		response = &papi.CardLimitBreakResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Items: []*ptransaction.UserItem{item}, Cards: []*ptransaction.UserCard{card},
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *CardServer) SkillEnhance(ctx context.Context, req *papi.CardSkillEnhanceRequest) (*papi.CardSkillEnhanceResponse, error) {
	if req.GetCardId() == "" || req.GetSkillId() == "" {
		return nil, viber.InvalidArgumentf("cardId and skillId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardSkillEnhanceResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		level, setLevel, ok := cardSkillLevel(card, req.SkillId)
		if !ok {
			return nil, viber.FailedPreconditionf("skill does not belong to the card or is not unlocked")
		}
		skill, err := opmaster.FindSkill(ctx, req.SkillId)
		if err != nil {
			return nil, err
		}
		next, err := nextSkillLevel(skill, level, card.Level)
		if err != nil {
			return nil, err
		}
		item, err := viber.ConsumePermanentItem(ctx, suid, cardSkillItemID, int64(next.RequiredItemAmount))
		if err != nil {
			return nil, err
		}
		setLevel(next.Level)
		if !card.Supported {
			setBaseSkillLevel(card, req.SkillId, next.Level)
		}
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		missions, err := viber.AdvanceUserMissions(
			ctx,
			suid,
			penum.MissionType_MissionType_CardSkillLevelUpCount,
			1,
			card.CardId,
			req.SkillId,
		)
		if err != nil {
			return nil, err
		}
		response = &papi.CardSkillEnhanceResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Items: []*ptransaction.UserItem{item}, Cards: []*ptransaction.UserCard{card}, Missions: missions,
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *CardServer) LiveAbilityEnhance(ctx context.Context, req *papi.CardLiveAbilityEnhanceRequest) (*papi.CardLiveAbilityEnhanceResponse, error) {
	if req.GetCardId() == "" || req.GetAbilityId() == "" {
		return nil, viber.InvalidArgumentf("cardId and abilityId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardLiveAbilityEnhanceResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		if card.LiveAbilityId != req.AbilityId {
			return nil, viber.FailedPreconditionf("live ability does not belong to the card or is not unlocked")
		}
		ability, err := opmaster.FindLiveAbility(ctx, req.AbilityId)
		if err != nil {
			return nil, err
		}
		next, err := nextLiveAbilityLevel(ability, card.LiveAbilityLevel, card.Level)
		if err != nil {
			return nil, err
		}
		item, err := viber.ConsumePermanentItem(ctx, suid, cardAbilityItemID, int64(next.RequiredItemAmount))
		if err != nil {
			return nil, err
		}
		card.LiveAbilityLevel = next.Level
		if !card.Supported {
			card.BaseLiveAbilityLevel = next.Level
		}
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		response = &papi.CardLiveAbilityEnhanceResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Items: []*ptransaction.UserItem{item}, Cards: []*ptransaction.UserCard{card},
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *CardServer) SetSupport(ctx context.Context, req *papi.CardSetSupportRequest) (*papi.CardSetSupportResponse, error) {
	if req.GetNumber() <= 0 || req.GetCardId() == "" {
		return nil, viber.InvalidArgumentf("positive number and cardId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardSetSupportResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		if card.Supported {
			return nil, viber.FailedPreconditionf("card is already assigned as support")
		}
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		if req.Number > user.CardSupportMaxNumber {
			return nil, viber.InvalidArgumentf("card support number must be between 1 and %d", user.CardSupportMaxNumber)
		}
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		if user.CardSupportLevel <= 0 || user.CardSupportLevel > setting.CardSupportLevelLimit {
			return nil, viber.FailedPreconditionf("user card support level is outside the master limit")
		}
		maxRelease, err := opmaster.FindMaxCardSupportRelease(ctx)
		if err != nil {
			return nil, err
		}
		if req.Number > maxRelease.Count+3 {
			return nil, viber.InvalidArgumentf("card support number exceeds the master slot limit")
		}
		supports, err := opuser.FindAllUserCardSupports(ctx, suid)
		if err != nil {
			return nil, err
		}
		if int32(len(supports)) >= user.CardSupportMaxNumber {
			return nil, viber.FailedPreconditionf("all card support slots are occupied")
		}
		for _, support := range supports {
			if support.Number == req.Number {
				return nil, viber.FailedPreconditionf("card support slot is already occupied")
			}
			if support.CardId == req.CardId {
				return nil, viber.FailedPreconditionf("card is already assigned as support")
			}
		}
		masterCard, err := opmaster.FindCard(ctx, card.CardId)
		if err != nil {
			return nil, err
		}
		supportLevel := user.CardSupportLevel
		attributes, err := usercard.CalcUserCardBaseAttr(ctx, masterCard, supportLevel, card.Rarity)
		if err != nil {
			return nil, err
		}
		photoCount, skillIDs, abilityIDs, err := usercard.CalcUnlocks(ctx, supportLevel, masterCard)
		if err != nil {
			return nil, err
		}
		support := &ptransaction.UserCardSupport{
			Number: req.Number, CardId: req.CardId,
			RemovableTime: time.Now().Add(time.Duration(setting.CardSupportRemovableMinutes) * time.Minute).UnixMilli(),
		}
		card.Supported = true
		card.Level = supportLevel
		card.Vocal, card.Dance, card.Visual, card.Stamina = attributes.Vocal, attributes.Dance, attributes.Visual, attributes.Stamina
		card.PhotoEquipableCount = photoCount
		card.SkillId1, card.SkillId2, card.SkillId3 = skillIDs[0], skillIDs[1], skillIDs[2]
		card.LiveAbilityId, card.ActivityAbilityId = abilityIDs[0], abilityIDs[1]
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserCardSupport(ctx, suid, support); err != nil {
			return nil, err
		}
		stories, err := newlyUnlockedCardStories(ctx, suid, masterCard, supportLevel)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertManyUserStories(ctx, suid, stories); err != nil {
			return nil, err
		}
		response = &papi.CardSetSupportResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Cards: []*ptransaction.UserCard{card}, CardSupports: []*ptransaction.UserCardSupport{support}, Stories: stories,
		})}
		return response, nil
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, viber.FailedPreconditionf("card support slot or card is already assigned")
		}
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func cardSkillLevel(card *ptransaction.UserCard, id string) (int32, func(int32), bool) {
	switch id {
	case card.SkillId1:
		return card.SkillLevel1, func(level int32) { card.SkillLevel1 = level }, id != ""
	case card.SkillId2:
		return card.SkillLevel2, func(level int32) { card.SkillLevel2 = level }, id != ""
	case card.SkillId3:
		return card.SkillLevel3, func(level int32) { card.SkillLevel3 = level }, id != ""
	case card.SkillId4:
		return card.SkillLevel4, func(level int32) { card.SkillLevel4 = level }, id != ""
	default:
		return 0, nil, false
	}
}

func setBaseSkillLevel(card *ptransaction.UserCard, id string, level int32) {
	switch id {
	case card.SkillId1:
		card.BaseSkillLevel1 = level
	case card.SkillId2:
		card.BaseSkillLevel2 = level
	case card.SkillId3:
		card.BaseSkillLevel3 = level
	case card.SkillId4:
		card.BaseSkillLevel4 = level
	}
}

func nextSkillLevel(skill *pmaster.Skill, current, cardLevel int32) (*pmaster.SkillLevel, error) {
	for _, level := range skill.Levels {
		if level.Level == current+1 {
			if level.RequiredCardLevel > cardLevel {
				return nil, viber.FailedPreconditionf("card level is too low for the next skill level")
			}
			if level.RequiredItemAmount <= 0 {
				return nil, fmt.Errorf("next skill level has no positive item cost")
			}
			return level, nil
		}
	}
	return nil, viber.FailedPreconditionf("skill is already at maximum level")
}

func nextLiveAbilityLevel(ability *pmaster.LiveAbility, current, cardLevel int32) (*pmaster.LiveAbilityLevel, error) {
	for _, level := range ability.Levels {
		if level.Level == current+1 {
			if level.RequiredCardLevel > cardLevel {
				return nil, viber.FailedPreconditionf("card level is too low for the next live ability level")
			}
			if level.RequiredItemAmount <= 0 {
				return nil, fmt.Errorf("next live ability level has no positive item cost")
			}
			return level, nil
		}
	}
	return nil, viber.FailedPreconditionf("live ability is already at maximum level")
}

func newlyUnlockedCardStories(ctx context.Context, suid string, masterCard *pmaster.Card, level int32) ([]*ptransaction.UserStory, error) {
	releases, err := usercard.CalcReleaseCounts(ctx, level, masterCard)
	if err != nil {
		return nil, err
	}
	storyCount := releases[penum.CardLevelReleaseType_CardLevelReleaseType_Story]
	if storyCount > int32(len(masterCard.Stories)) {
		storyCount = int32(len(masterCard.Stories))
	}
	existingStories, err := opuser.FindAllUserStories(ctx, suid)
	if err != nil {
		return nil, err
	}
	existingStoryIDs := make(map[string]struct{}, len(existingStories))
	for _, story := range existingStories {
		existingStoryIDs[story.StoryId] = struct{}{}
	}
	stories := make([]*ptransaction.UserStory, 0, storyCount)
	for _, story := range masterCard.Stories[:storyCount] {
		if _, exists := existingStoryIDs[story.StoryId]; exists {
			continue
		}
		stories = append(stories, &ptransaction.UserStory{
			StoryId: story.StoryId, StatusType: penum.StoryStatusType_StoryStatusType_Unread,
		})
	}
	return stories, nil
}
