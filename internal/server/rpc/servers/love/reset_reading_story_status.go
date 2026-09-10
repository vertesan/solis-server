package love

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *LoveServer) ResetReadingStoryStatus(ctx context.Context, req *papi.LoveResetReadingStoryStatusRequest) (*papi.LoveResetReadingStoryStatusResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.LoveResetReadingStoryStatusResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		_, state, love, err := loadLove(ctx, suid, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		love.IsReadingStory = false
		state.ActiveStoryEpisodeID = ""
		updated := &papi.UpdatedData{}
		if err := storeLove(ctx, state, love, updated); err != nil {
			return nil, err
		}
		response = &papi.LoveResetReadingStoryStatusResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
