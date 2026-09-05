package activity

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ActivityServer) LoadFanEvent(ctx context.Context, _ *papi.Empty) (*papi.ActivityLoadFanEventResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityLoadFanEventResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserFanEventRun(ctx, suid)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.FailedPrecondition, "no fan event is active")
			}
			return nil, err
		}
		state, err := viber.FindFanEventRunState(ctx, suid, progress)
		if err != nil {
			return nil, err
		}
		if _, err := viber.ReconcileFanEventProgress(ctx, suid, progress, state, time.Now().UnixMilli()); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserFanEventRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserFanEventRunState(ctx, state); err != nil {
			return nil, err
		}
		if err := syncActiveFanEvent(ctx, suid, progress); err != nil {
			return nil, err
		}
		response = &papi.ActivityLoadFanEventResponse{Progress: progress, CommonResponse: viber.NewCommonResponse(nil)}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
