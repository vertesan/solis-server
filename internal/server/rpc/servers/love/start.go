package love

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *LoveServer) Start(ctx context.Context, req *papi.LoveStartRequest) (*papi.LoveStartResponse, error) {
	return startLoveStory(ctx, viber.GetSuid(ctx).String(), req.GetStoryEpisodeId(), req.GetHintLevel(), false)
}
