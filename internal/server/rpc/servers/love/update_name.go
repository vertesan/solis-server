package love

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *LoveServer) UpdateName(ctx context.Context, req *papi.LoveUpdateNameRequest) (*papi.LoveUpdateNameResponse, error) {
	if err := validateLoveName(req.GetName()); err != nil {
		return nil, err
	}
	var response *papi.LoveUpdateNameResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		_, state, love, err := loadLove(ctx, viber.GetSuid(ctx).String(), req.GetLoveId())
		if err != nil {
			return nil, err
		}
		state.PlayerName = req.GetName()
		updated := &papi.UpdatedData{}
		if err := storeLove(ctx, state, love, updated); err != nil {
			return nil, err
		}
		response = &papi.LoveUpdateNameResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, err
}
