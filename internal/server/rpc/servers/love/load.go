package love

import (
	"context"
	"slices"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *LoveServer) Load(ctx context.Context, req *papi.LoveLoadRequest) (*papi.LoveLoadResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.LoveLoadResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		_, state, love, err := loadLove(ctx, suid, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		if err := validateLoveSave(ctx, love, req.GetSlotNumber()); err != nil {
			return nil, err
		}
		index := slices.IndexFunc(state.Saves, func(save umodel.UserLoveSave) bool { return save.Info.GetSlotNumber() == req.GetSlotNumber() })
		if index < 0 {
			return nil, status.Error(codes.NotFound, "save slot is empty")
		}
		routes, err := moshikoi.LoadRoutes(req.GetLoveId())
		if err != nil {
			return nil, err
		}
		save := state.Saves[index]
		if len(save.StoryEpisodeIDs) == 0 {
			return nil, viber.FailedPreconditionf("save slot has no story route")
		}
		if err := adjustLoveFavorability(love, routes.Favorability(save.Selections)-routes.Favorability(state.Selections)); err != nil {
			return nil, err
		}
		state.StoryEpisodeIDs, state.Selections = slices.Clone(save.StoryEpisodeIDs), slices.Clone(save.Selections)
		state.ActiveStoryEpisodeID = ""
		state.LastPlayTime = time.Now().UnixMilli()
		love.IsReadingStory = false
		updated := &papi.UpdatedData{}
		if err := storeLove(ctx, state, love, updated); err != nil {
			return nil, err
		}
		response = &papi.LoveLoadResponse{StoryEpisodeIds: state.StoryEpisodeIDs, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
