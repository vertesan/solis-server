package love

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *LoveServer) Reset(ctx context.Context, req *papi.LoveResetRequest) (*papi.LoveResetResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.LoveResetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		_, state, love, err := loadLove(ctx, suid, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		routes, err := moshikoi.LoadRoutes(req.GetLoveId())
		if err != nil {
			return nil, err
		}
		if err := adjustLoveFavorability(love, -routes.Favorability(state.Selections)); err != nil {
			return nil, err
		}
		state.StoryEpisodeIDs = []string{moshikoi.FirstEpisodeID(state.LoveID)}
		state.Selections = nil
		state.ActiveStoryEpisodeID = ""
		state.LastPlayTime = time.Now().UnixMilli()
		love.IsReadingStory = false
		updated := &papi.UpdatedData{}
		if err := storeLove(ctx, state, love, updated); err != nil {
			return nil, err
		}
		response = &papi.LoveResetResponse{StoryEpisodeIds: state.StoryEpisodeIDs, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
