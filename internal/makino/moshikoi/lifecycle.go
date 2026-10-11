package moshikoi

import (
	"context"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
)

// Extra series use their runtime ID for state/master ownership, but omit
// "love-extra-" in episode IDs and use "love-" in the underlying Story IDs.
func EpisodeIDPrefix(loveID string) string {
	return "love-event-story-" + strings.TrimPrefix(loveID, "love-extra-") + "-"
}

func SupportedStoryLoveID(storyID string) string {
	for _, id := range SupportedLoveIDs() {
		contentID := id
		if strings.HasPrefix(id, "love-extra-") {
			contentID = "love-" + strings.TrimPrefix(id, "love-extra-")
		}
		if strings.HasPrefix(storyID, "st-"+contentID+"-") {
			return id
		}
	}
	return ""
}

// InitializeUser creates unregistered runtime state. UserLove is only exposed
// after Register, matching the initial official event flow.
func InitializeUser(ctx context.Context, suid string) error {
	for _, id := range SupportedLoveIDs() {
		config, err := LoadConfig(id)
		if err != nil {
			return err
		}
		if err := opuser.UpsertUserLoveState(ctx, &umodel.UserLoveState{
			SUID: suid, LoveID: id, LastLessonRewardTime: config.StartTime,
			StoryEpisodeIDs: []string{FirstEpisodeID(id)},
		}); err != nil {
			return err
		}
	}
	return nil
}
