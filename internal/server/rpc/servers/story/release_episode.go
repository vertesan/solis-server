package story

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *StoryServer) ReleaseEpisode(ctx context.Context, req *papi.StoryReleaseEpisodeRequest) (*papi.StoryReleaseEpisodeResponse, error) {
	if req.GetStoryPartId() == "" || req.GetChapter() <= 0 || req.GetRoute() <= 0 || req.GetEpisode() <= 0 {
		return nil, viber.InvalidArgumentf("storyPartId and positive chapter, route and episode are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.StoryReleaseEpisodeResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		part, err := opmaster.FindStoryPart(ctx, req.GetStoryPartId())
		if err != nil {
			return nil, err
		}
		var episode *pmaster.StoryEpisode
		for _, chapter := range part.GetChapters() {
			if chapter.GetChapter() != req.GetChapter() || chapter.GetRoute() != req.GetRoute() {
				continue
			}
			for _, candidate := range chapter.GetEpisodes() {
				if candidate.GetEpisode() == req.GetEpisode() {
					episode = candidate
					break
				}
			}
		}
		if episode == nil || episode.GetStoryId() == "" {
			return nil, status.Error(codes.NotFound, "story episode does not exist")
		}
		story, err := opuser.FindUserStory(ctx, suid, episode.GetStoryId())
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		if story != nil && !story.GetIsInvalid() && story.GetStatusType() != penum.StoryStatusType_StoryStatusType_Unknown {
			return nil, viber.FailedPreconditionf("story episode is already unlocked")
		}
		if episode.GetIsReleased() || part.GetStoryEpisodeConsumptionId() == "" {
			return nil, viber.FailedPreconditionf("story episode is not unlocked by story points")
		}
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
		for _, condition := range []string{part.GetViewConditionId(), part.GetUnlockConditionId(), episode.GetViewConditionId(), episode.GetUnlockConditionId()} {
			ok, err := evaluator.Satisfied(condition)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, viber.FailedPreconditionf("story episode conditions are not satisfied")
			}
		}
		stories, err := opuser.FindAllUserStories(ctx, suid)
		if err != nil {
			return nil, err
		}
		owned := make(map[string]bool, len(stories))
		for _, value := range stories {
			owned[value.GetStoryId()] = !value.GetIsInvalid() && value.GetStatusType() != penum.StoryStatusType_StoryStatusType_Unknown
		}
		releaseCount := int32(1)
		seen := make(map[string]bool)
		for _, chapter := range part.GetChapters() {
			for _, candidate := range chapter.GetEpisodes() {
				if !candidate.GetIsReleased() && owned[candidate.GetStoryId()] && !seen[candidate.GetStoryId()] {
					releaseCount++
					seen[candidate.GetStoryId()] = true
				}
			}
		}
		cost, err := opmaster.FindStoryEpisodeConsumption(ctx, part.GetStoryEpisodeConsumptionId(), releaseCount)
		if err != nil {
			return nil, err
		}
		if cost.GetRequiredStoryPoint() <= 0 || part.GetPointType() == penum.PointType_PointType_Unknown {
			return nil, viber.FailedPreconditionf("story episode cost is invalid")
		}
		point, err := opuser.FindUserPoint(ctx, suid, part.GetPointType())
		if err != nil {
			return nil, err
		}
		if point.GetAmount() < int64(cost.GetRequiredStoryPoint()) {
			return nil, status.Error(codes.ResourceExhausted, "insufficient story points")
		}
		point.Amount -= int64(cost.GetRequiredStoryPoint())
		if err := opuser.UpsertUserPoint(ctx, suid, point); err != nil {
			return nil, err
		}
		if story == nil {
			story = &ptransaction.UserStory{StoryId: episode.GetStoryId()}
		}
		if story.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read {
			story.StatusType = penum.StoryStatusType_StoryStatusType_Unread
		}
		story.IsInvalid = false
		if err := opuser.UpsertUserStory(ctx, suid, story); err != nil {
			return nil, err
		}
		response = &papi.StoryReleaseEpisodeResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Stories: []*ptransaction.UserStory{story}, Points: []*ptransaction.UserPoint{point},
		})}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
