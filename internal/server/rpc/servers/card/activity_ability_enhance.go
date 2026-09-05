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
		next, err := nextCardActivityAbilityLevel(ability, card.ActivityAbilityLevel, card.Level)
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

func nextCardActivityAbilityLevel(ability *pmaster.ActivityAbility, current, cardLevel int32) (*pmaster.ActivityAbilityLevel, error) {
	for _, level := range ability.GetLevels() {
		if level.GetLevel() != current+1 {
			continue
		}
		if level.GetRequiredCardLevel() > cardLevel {
			return nil, viber.FailedPreconditionf("card level is too low for the next activity ability level")
		}
		if level.GetRequiredItemAmount() <= 0 {
			return nil, fmt.Errorf("next activity ability level has no positive item cost")
		}
		return level, nil
	}
	return nil, viber.FailedPreconditionf("activity ability is already at maximum level")
}
