package gacha

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *GachaServer) List(ctx context.Context, _ *papi.Empty) (*papi.GachaListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.GachaListResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		visible, err := viber.ReconcileCurrentGachaInfos(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		response = &papi.GachaListResponse{
			Gachas:         visible,
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
