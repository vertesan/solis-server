package user

import (
	"context"
	"errors"
	"slices"

	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func applyGodMoshikoi(ctx context.Context, suid string, counts map[string]int) error {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return err
	}
	for _, id := range moshikoi.SupportedLoveIDs() {
		catalog, err := moshikoi.LoadCatalog(ctx, id)
		if err != nil {
			return err
		}
		state, err := moshikoi.LoadState(ctx, suid, id)
		if err != nil {
			return err
		}
		state.Registered = true
		if state.PlayerName == "" {
			state.PlayerName = user.GetName()
		}
		for _, level := range catalog.Levels {
			state.Exp = max(state.Exp, level.GetRequiredExp())
		}
		if state.LastLessonRewardTime == 0 {
			state.LastLessonRewardTime = catalog.Config.StartTime
		}
		if len(state.StoryEpisodeIDs) == 0 {
			state.StoryEpisodeIDs = []string{moshikoi.FirstEpisodeID(id)}
		}
		love, err := moshikoi.UserLove(ctx, suid, id)
		if err != nil {
			return err
		}
		routes, err := moshikoi.LoadRoutes(id)
		if err != nil {
			return err
		}
		for _, ending := range routes.Endings {
			if !slices.Contains(love.UnlockedLoveStoryEndIDs, ending.ID) {
				love.UnlockedLoveStoryEndIDs = append(love.UnlockedLoveStoryEndIDs, ending.ID)
			}
		}
		// Preserve the active route, saves, chosen name and any higher progress.
		if err := moshikoi.StoreState(ctx, state, love); err != nil {
			return err
		}
		counts[uname.UserLoveState]++
		for _, episode := range catalog.Episodes {
			story, err := opuser.FindUserStory(ctx, suid, episode.GetStoryId())
			if err != nil {
				if !errors.Is(err, mongo.ErrNoDocuments) {
					return err
				}
				// God's preceding story pass normally creates these rows; use a
				// fresh record only when this event has a newly added master story.
				story = &ptransaction.UserStory{StoryId: episode.GetStoryId()}
			}
			story.StatusType = penum.StoryStatusType_StoryStatusType_Read
			story.IsInvalid = false
			if err := opuser.UpsertUserStory(ctx, suid, story); err != nil {
				return err
			}
		}
	}
	return nil
}
