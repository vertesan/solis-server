package love

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *LoveServer) PromoteLesson(ctx context.Context, req *papi.LovePromoteLessonRequest) (*papi.LovePromoteLessonResponse, error) {
	var response *papi.LovePromoteLessonResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		result, err := receiveLoveLesson(ctx, viber.GetSuid(ctx).String(), req.GetLoveId(), true)
		if err != nil {
			return nil, err
		}
		response = &papi.LovePromoteLessonResponse{Rewards: result.Rewards, ObtainExp: result.ObtainExp, TotalExp: result.TotalExp, LessonProgress: result.LessonProgress, CommonResponse: result.CommonResponse}
		return nil, nil
	})
	return response, err
}
