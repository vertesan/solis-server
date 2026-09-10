package love

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *LoveServer) Restart(ctx context.Context, req *papi.LoveRestartRequest) (*papi.LoveRestartResponse, error) {
	response, err := startLoveStory(ctx, viber.GetSuid(ctx).String(), req.GetStoryEpisodeId(), req.GetHintLevel(), true)
	if err != nil {
		return nil, err
	}
	return &papi.LoveRestartResponse{StoryChoiceGroupInfos: response.StoryChoiceGroupInfos, AdvBranchInfos: response.AdvBranchInfos, CommonResponse: response.CommonResponse}, nil
}
