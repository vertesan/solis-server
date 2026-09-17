package quest

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *QuestServer) GetRankingLiveResult(ctx context.Context, req *papi.QuestGetRankingLiveResultRequest) (*papi.QuestGetRankingLiveResultResponse, error) {
	if req.GetQuestId() == "" || req.GetUserId() == "" {
		return nil, viber.InvalidArgumentf("questId and userId are required")
	}
	suid := viber.GetSuid(ctx).String()
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if req.GetUserId() != public.GetPublicUserId() {
		return nil, status.Error(codes.NotFound, "live result is not available")
	}
	replay, err := opuser.FindQuestLiveResult(ctx, suid, req.GetQuestId(), opuser.QuestLiveReplayBest)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, status.Error(codes.NotFound, "live result is not available")
	}
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.QuestGetRankingLiveResultResponse{
		Result: replay.GetResult(), RankPatterns: replay.GetRankPatterns(), CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
