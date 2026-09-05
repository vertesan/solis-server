package photo

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CheckStoryEpisodeShooting(
	ctx context.Context,
	req *papi.PhotoCheckStoryEpisodeShootingRequest,
) (*papi.PhotoCheckStoryEpisodeShootingResponse, error) {
	storyID, err := resolvePhotoStoryEpisode(ctx, req.GetStoryPartId(), req.GetChapter(), req.GetRoute(), req.GetEpisode())
	if err != nil {
		return nil, err
	}
	story, err := opuser.FindUserStory(ctx, viber.GetSuid(ctx).String(), storyID)
	if err != nil {
		return nil, viber.PhotoOwnershipError("story", storyID, err)
	}
	if story.GetIsInvalid() ||
		story.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read &&
			!story.GetIsUseActivatePhotoShootingStoryTicket() {
		return nil, viber.FailedPreconditionf("story %q is not available for shooting", storyID)
	}
	return &papi.PhotoCheckStoryEpisodeShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
