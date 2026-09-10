package love

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *LoveServer) Finish(ctx context.Context, req *papi.LoveFinishRequest) (*papi.LoveFinishResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.LoveFinishResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, state, love, routes, err := loadLoveStory(ctx, suid, req.GetStoryEpisodeId())
		if err != nil {
			return nil, err
		}
		if !love.IsReadingStory || state.ActiveStoryEpisodeID != req.GetStoryEpisodeId() {
			return nil, viber.FailedPreconditionf("story episode is not being read")
		}
		if !slices.Contains(state.StoryEpisodeIDs, req.GetStoryEpisodeId()) {
			return nil, viber.FailedPreconditionf("active episode is not on the current route")
		}
		if _, err := loveHintLevel(catalog, state.Exp, req.GetHintLevel()); err != nil {
			return nil, err
		}
		selected, err := routes.SelectChoices(req.GetStoryEpisodeId(), req.GetSelectStoryChoiceInfos())
		if err != nil {
			return nil, viber.InvalidArgumentf("%s", err)
		}
		previousScore := routes.Favorability(state.Selections)
		truncateLoveRoute(state, req.GetStoryEpisodeId())
		state.Selections = append(state.Selections, selected...)
		for _, selection := range selected {
			if !slices.Contains(state.PastSelections, selection) {
				state.PastSelections = append(state.PastSelections, selection)
			}
		}
		next, endingID, err := routes.Resolve(req.GetStoryEpisodeId(), state.Selections)
		if err != nil {
			return nil, err
		}
		response = &papi.LoveFinishResponse{}
		if endingID != "" {
			ending := routes.Ending(endingID)
			if ending == nil {
				return nil, viber.FailedPreconditionf("story ending is not configured")
			}
			response.StoryEndName, response.EndRollAssetId = ending.Name, ending.EndRollAssetID
			if !slices.Contains(love.UnlockedLoveStoryEndIDs, endingID) {
				response.UnlockSave = len(love.UnlockedLoveStoryEndIDs) == 0
				love.UnlockedLoveStoryEndIDs = append(love.UnlockedLoveStoryEndIDs, endingID)
				if err := adjustLoveFavorability(love, ending.Favorability); err != nil {
					return nil, err
				}
			}
			state.Selections = nil
			state.StoryEpisodeIDs = []string{moshikoi.FirstEpisodeID(state.LoveID)}
		} else {
			state.StoryEpisodeIDs = append(state.StoryEpisodeIDs, next)
		}
		if err := adjustLoveFavorability(love, routes.Favorability(state.Selections)-previousScore); err != nil {
			return nil, err
		}
		love.IsReadingStory = false
		state.ActiveStoryEpisodeID = ""
		state.LastPlayTime = time.Now().UnixMilli()
		updated := &papi.UpdatedData{}
		episode := catalog.Episode(req.GetStoryEpisodeId())
		story, err := opuser.FindUserStory(ctx, suid, episode.GetStoryId())
		if errors.Is(err, mongo.ErrNoDocuments) {
			story = &ptransaction.UserStory{StoryId: episode.GetStoryId()}
		} else if err != nil {
			return nil, err
		}
		story.StatusType = penum.StoryStatusType_StoryStatusType_Read
		if err := opuser.UpsertUserStory(ctx, suid, story); err != nil {
			return nil, err
		}
		updated.Stories = append(updated.Stories, story)
		if err := storeLove(ctx, state, love, updated); err != nil {
			return nil, err
		}
		response.CommonResponse = viber.NewCommonResponse(updated)
		return nil, nil
	})
	return response, viber.GameplayError(err)
}

// A replay replaces choices from this point forward, not the permanent ending,
// mission or seen-choice records accumulated by earlier playthroughs.
func truncateLoveRoute(state *umodel.UserLoveState, episodeID string) {
	index := slices.Index(state.StoryEpisodeIDs, episodeID)
	state.StoryEpisodeIDs = slices.Clone(state.StoryEpisodeIDs[:index+1])
	previousEpisodes := state.StoryEpisodeIDs[:index]
	state.Selections = slices.DeleteFunc(state.Selections, func(selection umodel.LoveStorySelection) bool {
		return !slices.Contains(previousEpisodes, selection.StoryEpisodeID)
	})
}
