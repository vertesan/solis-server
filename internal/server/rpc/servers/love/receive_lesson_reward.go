package love

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *LoveServer) ReceiveLessonReward(ctx context.Context, req *papi.LoveReceiveLessonRewardRequest) (*papi.LoveReceiveLessonRewardResponse, error) {
	var response *papi.LoveReceiveLessonRewardResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		var err error
		response, err = receiveLoveLesson(ctx, viber.GetSuid(ctx).String(), req.GetLoveId(), false)
		return nil, err
	})
	return response, err
}
