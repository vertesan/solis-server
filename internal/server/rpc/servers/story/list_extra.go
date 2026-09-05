package story

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *StoryServer) ListExtra(ctx context.Context, _ *papi.Empty) (*papi.StoryListExtraResponse, error) {
	suid := viber.GetSuid(ctx).String()
	infos, err := opuser.FindAllUserExtraStoryPartInfos(ctx, suid)
	if err != nil {
		return nil, err
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Id < infos[j].Id })
	for _, info := range infos {
		sort.Slice(info.Stories, func(i, j int) bool { return info.Stories[i].Id < info.Stories[j].Id })
		for _, story := range info.Stories {
			sort.Slice(story.Episodes, func(i, j int) bool {
				return story.Episodes[i].Episode < story.Episodes[j].Episode
			})
		}
	}
	return &papi.StoryListExtraResponse{
		Infos:          infos,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
