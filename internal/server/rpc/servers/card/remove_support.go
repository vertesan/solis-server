package card

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CardServer) RemoveSupport(ctx context.Context, req *papi.CardRemoveSupportRequest) (*papi.CardRemoveSupportResponse, error) {
	if req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("a positive support number is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardRemoveSupportResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		supports, err := opuser.FindAllUserCardSupports(ctx, suid)
		if err != nil {
			return nil, err
		}
		var support *ptransaction.UserCardSupport
		for _, candidate := range supports {
			if candidate.Number == req.Number && candidate.CardId != "" {
				support = candidate
				break
			}
		}
		if support == nil {
			return nil, viber.FailedPreconditionf("card support slot is empty")
		}
		if support.RemovableTime > time.Now().UnixMilli() {
			return nil, viber.FailedPreconditionf("card support removal is on cooldown")
		}
		card, err := opuser.FindUserCard(ctx, suid, support.CardId)
		if err != nil {
			return nil, err
		}
		if !card.Supported {
			return nil, viber.FailedPreconditionf("card is not supported")
		}
		master, err := opmaster.FindCard(ctx, card.CardId)
		if err != nil {
			return nil, err
		}
		rewards, err := cardAbilityTrainingRefunds(ctx, master, card, false)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if err := refundCardTrainingItems(ctx, suid, rewards, updated); err != nil {
			return nil, err
		}
		card.Supported = false
		card.Level = card.BaseLevel
		card.Vocal, card.Dance, card.Visual, card.Stamina = card.BaseVocal, card.BaseDance, card.BaseVisual, card.BaseStamina
		card.PhotoEquipableCount = card.BasePhotoEquipableCount
		card.SkillId1, card.SkillId2, card.SkillId3, card.SkillId4 = card.BaseSkillId1, card.BaseSkillId2, card.BaseSkillId3, card.BaseSkillId4
		card.SkillLevel1, card.SkillLevel2, card.SkillLevel3, card.SkillLevel4 = card.BaseSkillLevel1, card.BaseSkillLevel2, card.BaseSkillLevel3, card.BaseSkillLevel4
		card.LiveAbilityId, card.ActivityAbilityId = card.BaseLiveAbilityId, card.BaseActivityAbilityId
		card.LiveAbilityLevel, card.ActivityAbilityLevel = card.BaseLiveAbilityLevel, card.BaseActivityAbilityLevel
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserCardSupport(ctx, suid, support.Number); err != nil {
			return nil, err
		}
		unlocked, locked, err := viber.UpdateCardStoryUnlocks(ctx, suid, master, card.Level, card.EvolutionLevel)
		if err != nil {
			return nil, err
		}
		support.CardId = ""
		updated.Cards = []*ptransaction.UserCard{card}
		updated.CardSupports = []*ptransaction.UserCardSupport{support}
		updated.Stories = unlocked
		common := viber.NewCommonResponse(updated)
		common.DeletedData.Stories = locked
		response = &papi.CardRemoveSupportResponse{Rewards: rewards, CommonResponse: common}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
