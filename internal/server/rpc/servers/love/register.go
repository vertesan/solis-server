package love

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *LoveServer) Register(ctx context.Context, req *papi.LoveRegisterRequest) (*papi.LoveRegisterResponse, error) {
	if err := validateLoveName(req.GetName()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.LoveRegisterResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, err := moshikoi.LoadCatalog(ctx, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		unlocked, err := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli()).Satisfied(catalog.Config.UnlockConditionID)
		if err != nil {
			return nil, err
		}
		if !unlocked {
			return nil, status.Error(codes.FailedPrecondition, "love event is locked")
		}
		state, err := moshikoi.LoadState(ctx, suid, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		if state.Registered {
			return nil, status.Error(codes.AlreadyExists, "love player is already registered")
		}
		love, err := moshikoi.UserLove(ctx, suid, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		state.Registered = true
		state.PlayerName = req.GetName()
		state.LastPlayTime = time.Now().UnixMilli()
		if state.LastLessonRewardTime == 0 {
			state.LastLessonRewardTime = catalog.Config.StartTime
		}
		updated := &papi.UpdatedData{}
		if err := storeLove(ctx, state, love, updated); err != nil {
			return nil, err
		}
		response = &papi.LoveRegisterResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, err
}
