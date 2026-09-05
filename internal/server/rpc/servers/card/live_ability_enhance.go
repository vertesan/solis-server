package card

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

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
