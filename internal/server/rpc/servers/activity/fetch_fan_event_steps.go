package activity

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ActivityServer) FetchFanEventSteps(ctx context.Context, req *papi.ActivityFetchFanEventStepsRequest) (*papi.ActivityFetchFanEventStepsResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityFetchFanEventStepsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserFanEventRun(ctx, suid)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.FailedPrecondition, "no fan event is active")
			}
			return nil, err
		}
		if len(req.GetSteps()) != len(progress.GetCharacters()) {
			return nil, status.Error(codes.InvalidArgument, "one step cursor is required per character")
		}
		state, err := viber.FindFanEventRunState(ctx, suid, progress)
		if err != nil {
			return nil, err
		}
		characterSteps, err := issueFanEventSteps(ctx, progress, state, req.GetSteps())
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserFanEventRunState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.ActivityFetchFanEventStepsResponse{
			CharacterSteps: characterSteps,
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
