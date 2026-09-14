package quest

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *QuestServer) ListLatestClear(ctx context.Context, req *papi.QuestListLatestClearRequest) (*papi.QuestListLatestClearResponse, error) {
	if req.GetQuestId() == "" {
		return nil, viber.InvalidArgumentf("questId is required")
	}
	if _, err := opmaster.FindQuest(ctx, req.GetQuestId()); err != nil {
		return nil, viber.GameplayError(err)
	}
	// The private server does not publish other managers' live history.
	return &papi.QuestListLatestClearResponse{
		Clears: []*papi.QuestClearInfo{}, CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
