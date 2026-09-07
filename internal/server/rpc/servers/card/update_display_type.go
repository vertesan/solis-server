package card

import (
	"context"

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
		if err := viber.ValidateCardDisplayType(ctx, card, req.DisplayType); err != nil {
			return nil, err
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
