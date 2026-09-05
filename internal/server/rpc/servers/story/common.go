package story

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func reconcileNarrativeSchedulesAfterStory(
	ctx context.Context,
	suid string,
	now int64,
	storyID string,
	updated *papi.UpdatedData,
) error {
	return viber.ReconcileNarrativeSchedules(ctx, suid, now, updated, viber.NarrativeUnlockReadStory, storyID)
}

func readUserStory(
	ctx context.Context,
	suid string,
	storyId string,
	allowCreate bool,
) ([]*ptransaction.RewardResult, *papi.UpdatedData, error) {
	master, err := opmaster.FindStory(ctx, storyId)
	if err != nil {
		return nil, nil, err
	}
	story, err := opuser.FindUserStory(ctx, suid, storyId)
	if err != nil {
		if !allowCreate || !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil, err
		}
		story = &ptransaction.UserStory{StoryId: storyId, StatusType: penum.StoryStatusType_StoryStatusType_Unread}
	}
	if story.IsInvalid {
		return nil, nil, status.Error(codes.PermissionDenied, "story is unavailable")
	}
	updated := &papi.UpdatedData{}
	if story.StatusType == penum.StoryStatusType_StoryStatusType_Read {
		return []*ptransaction.RewardResult{}, updated, nil
	}
	if story.StatusType != penum.StoryStatusType_StoryStatusType_Unread {
		return nil, nil, status.Error(codes.PermissionDenied, "story is locked")
	}
	story.StatusType = penum.StoryStatusType_StoryStatusType_Read
	if err := opuser.UpsertUserStory(ctx, suid, story); err != nil {
		return nil, nil, err
	}
	updated.Stories = append(updated.Stories, story)
	rewards, err := grantStoryReward(ctx, suid, master, updated)
	if err != nil {
		return nil, nil, err
	}
	if err := reconcileNarrativeSchedulesAfterStory(ctx, suid, time.Now().UnixMilli(), storyId, updated); err != nil {
		return nil, nil, err
	}
	return rewards, updated, nil
}

func grantStoryReward(
	ctx context.Context,
	suid string,
	story *pmaster.Story,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if story.RewardId == "" {
		return []*ptransaction.RewardResult{}, nil
	}
	resources, err := viber.ParseAutoReward(story.RewardId)
	if err != nil {
		return nil, err
	}
	if resources == nil {
		reward, err := viber.FindMasterReward(ctx, story.RewardId)
		if err != nil {
			return nil, err
		}
		resources = reward.Resources
	}
	results := make([]*ptransaction.RewardResult, 0, len(resources))
	for _, resource := range resources {
		if resource.Amount <= 0 {
			return nil, fmt.Errorf("story %q has a non-positive reward", story.Id)
		}
		granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, granted...)
	}
	return results, nil
}
