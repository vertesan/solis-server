package story

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *StoryServer) ReadExtra(ctx context.Context, req *papi.StoryReadExtraRequest) (*papi.StoryReadExtraResponse, error) {
	if req.GetExtraStoryId() == "" || req.GetEpisode() <= 0 {
		return nil, viber.InvalidArgumentf("extraStoryId and a positive episode are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.StoryReadExtraResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		master, err := opmaster.FindExtraStory(ctx, req.ExtraStoryId)
		if err != nil {
			return nil, err
		}
		storyId := ""
		for _, episode := range master.Episodes {
			if episode.Episode == req.Episode {
				storyId = episode.StoryId
				break
			}
		}
		if storyId == "" {
			return nil, viber.InvalidArgumentf("extra story episode does not exist")
		}
		part, err := opuser.FindUserExtraStoryPartInfo(ctx, suid, master.ExtraStoryPartId)
		if err != nil {
			return nil, err
		}
		if !extraEpisodeUnlocked(part, req.ExtraStoryId, req.Episode) {
			return nil, status.Error(codes.PermissionDenied, "extra story episode is locked")
		}
		rewards, updated, err := readUserStory(ctx, suid, storyId, true)
		if err != nil {
			return nil, err
		}
		response = &papi.StoryReadExtraResponse{Rewards: rewards, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func extraEpisodeUnlocked(info *papi.ExtraStoryPartInfo, storyId string, episode int32) bool {
	if !info.Unlocked {
		return false
	}
	for _, story := range info.Stories {
		if story.Id != storyId || !story.Unlocked {
			continue
		}
		for _, value := range story.Episodes {
			if value.Episode == episode {
				return value.Unlocked
			}
		}
	}
	return false
}
