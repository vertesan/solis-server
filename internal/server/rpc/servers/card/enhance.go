package card

import (
	"context"
	"fmt"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/salary"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
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
		if err := opuser.UpsertUser(ctx, suid, user); err != nil {
			return nil, err
		}
		managerLevel, err := opmaster.FindManagerLevel(ctx, user.ManagerLevel)
		if err != nil {
			return nil, err
		}
		rarityLevelLimit := int64(cardRarity.LevelLimit)
		if card.Rank > 0 {
			rank, err := opmaster.FindCardRank(ctx, card.Rank)
			if err != nil {
				return nil, err
			}
			rarityLevelLimit += int64(rank.LevelLimitUpAmount)
		}
		if int64(req.Level) > rarityLevelLimit || req.Level > managerLevel.MaxCardLevel {
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
		photoCount, skillIDs, abilityIDs, err := usercard.CalcUnlocksWithEvolution(
			ctx, req.Level, card.GetEvolutionLevel(), masterCard,
		)
		if err != nil {
			return nil, err
		}
		stories, _, err := viber.UpdateCardStoryUnlocks(ctx, suid, masterCard, req.Level, card.EvolutionLevel)
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
		card.SkillId4, card.BaseSkillId4 = skillIDs[3], skillIDs[3]
		card.LiveAbilityId, card.ActivityAbilityId = abilityIDs[0], abilityIDs[1]
		card.BaseLiveAbilityId, card.BaseActivityAbilityId = abilityIDs[0], abilityIDs[1]
		card.LastEnhanceTime = time.Now().UnixMilli()
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
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
		characterMissions, err := viber.ReachCharacterCardTrainingMissions(ctx, suid, masterCard.CharacterId)
		if err != nil {
			return nil, err
		}
		missions = append(missions, characterMissions...)
		updated := &papi.UpdatedData{
			Items:    []*ptransaction.UserItem{item},
			Cards:    []*ptransaction.UserCard{card},
			Stories:  stories,
			Missions: missions,
		}
		if _, err := viber.ReconcileCardSupportLevel(ctx, suid, user, updated); err != nil {
			return nil, err
		}
		if err := salary.RefreshSnapshot(ctx, suid, time.Now()); err != nil {
			return nil, err
		}
		response = &papi.CardEnhanceResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
