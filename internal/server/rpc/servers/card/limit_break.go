package card

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

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
		card.Rarity = req.Rarity
		card.RarityTotalExp = targetRarity.RequiredExp
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
