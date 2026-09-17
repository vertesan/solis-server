package quest

import (
	"context"
	"github.com/vertesan/solis-server/internal/proto/papi"
)

func (s *QuestServer) Start(ctx context.Context, req *papi.QuestStartRequest) (*papi.QuestStartResponse, error) {
	return startQuest(ctx, req, nil)
}
