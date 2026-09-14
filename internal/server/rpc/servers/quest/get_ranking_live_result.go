package quest

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *QuestServer) GetRankingLiveResult(_ context.Context, req *papi.QuestGetRankingLiveResultRequest) (*papi.QuestGetRankingLiveResultResponse, error) {
	if req.GetQuestId() == "" || req.GetUserId() == "" {
		return nil, viber.InvalidArgumentf("questId and userId are required")
	}
	// Rankings explicitly advertise that replay detail is unavailable until
	// Quest.Start can produce and persist an actual live result.
	return nil, status.Error(codes.NotFound, "live result is not available")
}
