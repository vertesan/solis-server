package quest

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *QuestServer) GetLatestClearLiveResult(_ context.Context, req *papi.QuestGetLatestClearLiveResultRequest) (*papi.QuestGetLatestClearLiveResultResponse, error) {
	if req.GetQuestId() == "" || req.GetUserId() == "" {
		return nil, viber.InvalidArgumentf("questId and userId are required")
	}
	// ListLatestClear is empty. Do not fabricate a replay or look up another
	// manager when a stale client directly requests an old public result.
	return nil, status.Error(codes.NotFound, "live result is not available")
}
