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

func (s *StoryServer) ReadEvent(ctx context.Context, req *papi.StoryReadEventRequest) (*papi.StoryReadEventResponse, error) {
	if req.GetEventStoryId() == "" || req.GetEpisode() <= 0 {
		return nil, viber.InvalidArgumentf("eventStoryId and a positive episode are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.StoryReadEventResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		master, err := opmaster.FindEventStory(ctx, req.EventStoryId)
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
			return nil, viber.InvalidArgumentf("event story episode does not exist")
		}
		info, err := opuser.FindUserEventStoryInfo(ctx, suid, req.EventStoryId)
		if err != nil {
			return nil, err
		}
		if !eventEpisodeUnlocked(info, req.Episode) {
			return nil, status.Error(codes.PermissionDenied, "event story episode is locked")
		}
		rewards, updated, err := readUserStory(ctx, suid, storyId, true)
		if err != nil {
			return nil, err
		}
		response = &papi.StoryReadEventResponse{Rewards: rewards, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func eventEpisodeUnlocked(info *papi.EventStoryInfo, episode int32) bool {
	for _, value := range info.Episodes {
		if value.Episode == episode {
			return value.Unlocked
		}
	}
	return false
}
