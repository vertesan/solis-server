package excursion

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ExcursionServer) Start(ctx context.Context, req *papi.ExcursionStartRequest) (*papi.ExcursionStartResponse, error) {
	if req.GetCharacterId() == "" || req.GetExcursionPlaceId() == "" {
		return nil, status.Error(codes.InvalidArgument, "characterId and excursionPlaceId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ExcursionStartResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := requireExcursionUnlocked(ctx, suid); err != nil {
			return nil, err
		}
		if _, err := opuser.FindUserCharacter(ctx, suid, req.GetCharacterId()); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.PermissionDenied, "character is not owned")
			}
			return nil, err
		}
		if _, err := opmaster.FindExcursionPlaceSetting(ctx, req.GetCharacterId(), req.GetExcursionPlaceId()); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.NotFound, "excursion place is unavailable for character")
			}
			return nil, err
		}
		if _, err := opmaster.FindExcursionPlace(ctx, req.GetExcursionPlaceId()); err != nil {
			return nil, err
		}
		state, err := findExcursionStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		if state.ActiveCharacter != "" {
			return nil, status.Error(codes.FailedPrecondition, "an excursion is already active")
		}
		if state.OfferedCharacter != req.GetCharacterId() ||
			!slices.Contains(state.OfferedPlaceIds, req.GetExcursionPlaceId()) {
			return nil, status.Error(codes.FailedPrecondition, "excursion place was not offered")
		}
		state.ActiveCharacter = req.GetCharacterId()
		state.ActivePlaceId = req.GetExcursionPlaceId()
		state.StartedTime = time.Now().UnixMilli()
		state.OfferedCharacter = ""
		state.OfferedPlaceIds = []string{}
		if err := opuser.UpsertUserExcursionState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.ExcursionStartResponse{
			RewardResults:  []*ptransaction.RewardResult{},
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
