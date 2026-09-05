package story

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *StoryServer) Read(ctx context.Context, req *papi.StoryReadRequest) (*papi.StoryReadResponse, error) {
	if req.GetStoryId() == "" {
		return nil, viber.InvalidArgumentf("storyId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.StoryReadResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		rewards, updated, err := readUserStory(ctx, suid, req.StoryId, false)
		if err != nil {
			return nil, err
		}
		response = &papi.StoryReadResponse{Rewards: rewards, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
