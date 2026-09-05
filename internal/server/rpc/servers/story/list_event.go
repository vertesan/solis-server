package story

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *StoryServer) ListEvent(ctx context.Context, _ *papi.Empty) (*papi.StoryListEventResponse, error) {
	suid := viber.GetSuid(ctx).String()
	infos, err := opuser.FindAllUserEventStoryInfos(ctx, suid)
	if err != nil {
		return nil, err
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Id < infos[j].Id })
	for _, info := range infos {
		sort.Slice(info.Episodes, func(i, j int) bool { return info.Episodes[i].Episode < info.Episodes[j].Episode })
	}
	return &papi.StoryListEventResponse{
		Infos:          infos,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
