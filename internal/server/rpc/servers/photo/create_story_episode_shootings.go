package photo

import (
	"context"
	"fmt"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CreateStoryEpisodeShootings(
	ctx context.Context,
	req *papi.PhotoCreateStoryEpisodeShootingsRequest,
) (*papi.PhotoCreateStoryEpisodeShootingsResponse, error) {
	if len(req.GetCreateShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateStoryEpisodeShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckStoryEpisodeShooting(ctx, &papi.PhotoCheckStoryEpisodeShootingRequest{
			StoryPartId: req.GetStoryPartId(),
			Chapter:     req.GetChapter(),
			Route:       req.GetRoute(),
			Episode:     req.GetEpisode(),
		}); err != nil {
			return nil, err
		}
		part, err := opmaster.FindStoryPart(ctx, req.GetStoryPartId())
		if err != nil {
			return nil, err
		}
		storyID, err := resolvePhotoStoryEpisode(ctx, req.GetStoryPartId(), req.GetChapter(), req.GetRoute(), req.GetEpisode())
		if err != nil {
			return nil, err
		}
		candidates, err := photoStoryCharacters(ctx, storyID)
		if err != nil {
			return nil, err
		}
		scene := viber.PhotoShootingScene{
			PlaceName:             fmt.Sprintf("%s %d章 %d話", part.GetName(), req.GetChapter(), req.GetEpisode()),
			EventName:             storyPartEventName(part.GetName()),
			CandidateCharacterIDs: candidates,
			StoryID:               storyID,
			StoryShooting:         true,
		}
		result, err := viber.CreateUserPhotoShootings(
			ctx, suid, scene, viber.CommonPhotoParams(req.GetCreateShootingParams()), req.GetManualCount(),
		)
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoCreateStoryEpisodeShootingsResponse{
			Photos: result.Photos, IsGift: result.IsGift, CommonResponse: viber.NewCommonResponse(result.Updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func storyPartEventName(name string) string {
	if strings.HasSuffix(name, "編") {
		return name
	}
	return name + "編"
}
