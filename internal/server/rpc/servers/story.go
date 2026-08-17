package servers

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

type StoryServer struct {
	papi.UnimplementedStoryServer
}

func (s *StoryServer) ListEvent(ctx context.Context, _ *papi.Empty) (*papi.StoryListEventResponse, error) {
	suid := viber.GetSuid(ctx).String()
	infos, err := opuser.FindAllUserEventStoryInfos(ctx, suid)
	if err != nil {
		return nil, err
	}
	return &papi.StoryListEventResponse{
		Infos:          infos,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
