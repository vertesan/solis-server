package servers

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func (s *CardServer) ActivityAbilityEnhance(ctx context.Context, req *papi.CardActivityAbilityEnhanceRequest) (*papi.CardActivityAbilityEnhanceResponse, error) {
	if req.GetCardId() == "" || req.GetAbilityId() == "" {
		return nil, viber.InvalidArgumentf("cardId and abilityId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardActivityAbilityEnhanceResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		if card.ActivityAbilityId != req.AbilityId {
			return nil, viber.FailedPreconditionf("activity ability does not belong to the card or is not unlocked")
		}
		ability, err := opmaster.FindActivityAbility(ctx, req.AbilityId)
		if err != nil {
			return nil, err
		}
		next, err := viber.NextCardActivityAbilityLevel(ability, card.ActivityAbilityLevel, card.Level)
		if err != nil {
			return nil, err
		}
		item, err := viber.ConsumePermanentItem(ctx, suid, cardAbilityItemID, int64(next.RequiredItemAmount))
		if err != nil {
			return nil, err
		}
		card.ActivityAbilityLevel = next.Level
		if !card.Supported {
			card.BaseActivityAbilityLevel = next.Level
		}
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		response = &papi.CardActivityAbilityEnhanceResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Items: []*ptransaction.UserItem{item}, Cards: []*ptransaction.UserCard{card},
		})}
		return response, nil
	})
	return response, viber.GameplayError(err)
}

func (s *CardServer) Reset(ctx context.Context, req *papi.CardResetRequest) (*papi.CardResetResponse, error) {
	if req.GetCardId() == "" {
		return nil, viber.InvalidArgumentf("cardId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardResetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		if card.Supported {
			return nil, viber.FailedPreconditionf("remove card support before resetting the card")
		}
		master, err := opmaster.FindCard(ctx, card.CardId)
		if err != nil {
			return nil, err
		}
		initialLevel, err := opmaster.FindCardLevel(ctx, 1)
		if err != nil {
			return nil, err
		}
		initialRank, err := opmaster.FindCardRank(ctx, 0)
		if err != nil {
			return nil, err
		}
		trainingRefunds, err := viber.CardAbilityTrainingRefunds(ctx, master, card, true)
		if err != nil {
			return nil, err
		}
		expRefund := card.TotalExp - initialLevel.RequiredExp
		rankRefund := int64(card.RankTotalExp) - int64(initialRank.RequiredExp)
		if expRefund < 0 || rankRefund < 0 {
			return nil, viber.FailedPreconditionf("card training EXP is invalid")
		}
		if expRefund == 0 && rankRefund == 0 && len(trainingRefunds) == 0 {
			return nil, viber.FailedPreconditionf("card has no training to reset")
		}
		var rewards []*ptransaction.Reward
		if expRefund > 0 {
			rewards = append(rewards, &ptransaction.Reward{ResourceType: penum.ResourceType_ResourceType_Item, ResourceId: cardEnhanceItemID, Amount: expRefund})
		}
		rewards = append(rewards, trainingRefunds...)
		if rankRefund > 0 {
			rewards = append(rewards, &ptransaction.Reward{ResourceType: penum.ResourceType_ResourceType_Item, ResourceId: "item-card_rank", Amount: rankRefund})
		}
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		if setting.CardResetStoneAmount < 0 {
			return nil, fmt.Errorf("card reset stone cost is negative")
		}
		updated := &papi.UpdatedData{}
		if setting.CardResetStoneAmount > 0 {
			balance, err := opuser.FindUserBalance(ctx, suid)
			if err != nil {
				return nil, err
			}
			if int64(balance.FreeBalance)+int64(balance.PaidBalance) < int64(setting.CardResetStoneAmount) {
				return nil, viber.FailedPreconditionf("insufficient dia to reset the card")
			}
			freeCost := min(balance.FreeBalance, setting.CardResetStoneAmount)
			balance.FreeBalance -= freeCost
			balance.PaidBalance -= setting.CardResetStoneAmount - freeCost
			if err := opuser.UpsertUserBalance(ctx, suid, balance); err != nil {
				return nil, err
			}
			updated.Balance = balance
		}
		if err := viber.RefundCardTrainingItems(ctx, suid, rewards, updated); err != nil {
			return nil, err
		}
		attributes, err := usercard.CalcUserCardBaseAttr(ctx, master, 1, card.Rarity)
		if err != nil {
			return nil, err
		}
		photos, skills, abilities, err := usercard.CalcUnlocksWithEvolution(ctx, 1, card.EvolutionLevel, master)
		if err != nil {
			return nil, err
		}
		card.TotalExp, card.RankTotalExp = initialLevel.RequiredExp, initialRank.RequiredExp
		card.Level, card.BaseLevel, card.Rank = 1, 1, initialRank.Rank
		card.Vocal, card.BaseVocal = attributes.Vocal, attributes.Vocal
		card.Dance, card.BaseDance = attributes.Dance, attributes.Dance
		card.Visual, card.BaseVisual = attributes.Visual, attributes.Visual
		card.Stamina, card.BaseStamina = attributes.Stamina, attributes.Stamina
		card.SkillId1, card.BaseSkillId1, card.SkillLevel1, card.BaseSkillLevel1 = skills[0], skills[0], 1, 1
		card.SkillId2, card.BaseSkillId2, card.SkillLevel2, card.BaseSkillLevel2 = skills[1], skills[1], 1, 1
		card.SkillId3, card.BaseSkillId3, card.SkillLevel3, card.BaseSkillLevel3 = skills[2], skills[2], 1, 1
		card.SkillId4, card.BaseSkillId4 = skills[3], skills[3]
		fourthSkillLevel := int32(0)
		if master.SkillId4 != "" || card.BaseSkillLevel4 > 0 {
			fourthSkillLevel = 1
		}
		card.SkillLevel4, card.BaseSkillLevel4 = fourthSkillLevel, fourthSkillLevel
		card.LiveAbilityId, card.BaseLiveAbilityId, card.LiveAbilityLevel, card.BaseLiveAbilityLevel = abilities[0], abilities[0], 1, 1
		card.ActivityAbilityId, card.BaseActivityAbilityId, card.ActivityAbilityLevel, card.BaseActivityAbilityLevel = abilities[1], abilities[1], 1, 1
		card.PhotoEquipableCount, card.BasePhotoEquipableCount = photos, photos
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		unlocked, locked, err := viber.UpdateCardStoryUnlocks(ctx, suid, master, card.Level, card.EvolutionLevel)
		if err != nil {
			return nil, err
		}
		updated.Cards, updated.Stories = []*ptransaction.UserCard{card}, unlocked
		common := viber.NewCommonResponse(updated)
		common.DeletedData.Stories = locked
		response = &papi.CardResetResponse{CommonResponse: common, Rewards: rewards}
		return response, nil
	})
	return response, viber.GameplayError(err)
}
