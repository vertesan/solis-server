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

func (s *QuestServer) GetLatestClearLiveResult(ctx context.Context, req *papi.QuestGetLatestClearLiveResultRequest) (*papi.QuestGetLatestClearLiveResultResponse, error) {
	if req.GetQuestId() == "" || req.GetUserId() == "" {
		return nil, viber.InvalidArgumentf("questId and userId are required")
	}
	suid := viber.GetSuid(ctx).String()
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	// The public latest-clear list stays empty. A direct own-user replay request
	// is useful, but must never expose another manager's stored performance.
	if req.GetUserId() != public.GetPublicUserId() {
		return nil, status.Error(codes.NotFound, "live result is not available")
	}
	response, err := opuser.FindQuestLiveResult(ctx, suid, req.GetQuestId(), opuser.QuestLiveReplayLatestClear)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, status.Error(codes.NotFound, "live result is not available")
	}
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	response.RankPatterns, err = questReplayRankPatterns(ctx, req.GetQuestId(), response.GetRankPatterns())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	response.CommonResponse = viber.NewCommonResponse(nil)
	return response, nil
}
