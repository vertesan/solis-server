package love

import (
	"context"
	"slices"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *LoveServer) Save(ctx context.Context, req *papi.LoveSaveRequest) (*papi.LoveSaveResponse, error) {
	suid := viber.GetSuid(ctx).String()
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		_, state, love, err := loadLove(ctx, suid, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		if err := validateLoveSave(ctx, love, req.GetSlotNumber()); err != nil {
			return nil, err
		}
		if love.IsReadingStory {
			return nil, viber.FailedPreconditionf("finish reading before saving")
		}
		index := slices.IndexFunc(state.Saves, func(save umodel.UserLoveSave) bool { return save.Info.GetSlotNumber() == req.GetSlotNumber() })
		name := ""
		if index >= 0 {
			name = state.Saves[index].Info.GetName()
		}
		save := umodel.UserLoveSave{
			Info:            &papi.LoveSaveDataInfo{SlotNumber: req.GetSlotNumber(), SaveTime: time.Now().UnixMilli(), LatestStoryEpisodeId: state.StoryEpisodeIDs[len(state.StoryEpisodeIDs)-1], Name: name},
			StoryEpisodeIDs: slices.Clone(state.StoryEpisodeIDs), Selections: slices.Clone(state.Selections),
		}
		if index >= 0 {
			state.Saves[index] = save
		} else {
			state.Saves = append(state.Saves, save)
		}
		return nil, opuser.UpsertUserLoveState(ctx, state)
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.LoveSaveResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
