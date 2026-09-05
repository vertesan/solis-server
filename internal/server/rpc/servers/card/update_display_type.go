package card

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CardServer) UpdateDisplayType(ctx context.Context, req *papi.CardUpdateDisplayTypeRequest) (*papi.CardUpdateDisplayTypeResponse, error) {
	if req.GetCardId() == "" || req.GetDisplayType() < penum.CardDisplayType_CardDisplayType_BeforeLimitBreak || req.GetDisplayType() > penum.CardDisplayType_CardDisplayType_AfterEvolution {
		return nil, viber.InvalidArgumentf("cardId and a valid displayType are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardUpdateDisplayTypeResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		images, err := opmaster.FindCardImages(ctx, card.CardId)
		if err != nil {
			return nil, err
		}
		available := false
		for _, image := range images {
			if image.DisplayType == req.DisplayType {
				available = true
				break
			}
		}
		if !available {
			return nil, viber.FailedPreconditionf("card has no image for the requested display type")
		}
		if req.DisplayType == penum.CardDisplayType_CardDisplayType_AfterLimitBreak && card.Rarity < 5 {
			return nil, viber.FailedPreconditionf("card must reach rarity 5 for its limit-break image")
		}
		if req.DisplayType == penum.CardDisplayType_CardDisplayType_AfterEvolution && card.EvolutionLevel <= 0 {
			return nil, viber.FailedPreconditionf("card must be evolved for its evolution image")
		}
		card.DisplayType = req.DisplayType
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		response = &papi.CardUpdateDisplayTypeResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Cards: []*ptransaction.UserCard{card},
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
