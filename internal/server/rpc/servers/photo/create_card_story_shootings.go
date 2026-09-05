package photo

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CreateCardStoryShootings(
	ctx context.Context,
	req *papi.PhotoCreateCardStoryShootingsRequest,
) (*papi.PhotoCreateCardStoryShootingsResponse, error) {
	if len(req.GetCreateShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateCardStoryShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckCardStoryShooting(ctx, &papi.PhotoCheckCardStoryShootingRequest{
			CardId: req.GetCardId(), Number: req.GetNumber(),
		}); err != nil {
			return nil, err
		}
		card, err := opmaster.FindCard(ctx, req.GetCardId())
		if err != nil {
			return nil, err
		}
		storyID := card.GetStories()[req.GetNumber()-1].GetStoryId()
		candidates, err := photoStoryCharacters(ctx, storyID)
		if err != nil {
			return nil, err
		}
		candidates = appendUniquePhotoString(candidates, card.GetCharacterId())
		scene := viber.PhotoShootingScene{
			PlaceName:             fmt.Sprintf("%s %d話", card.GetName(), req.GetNumber()),
			EventName:             "アイドル編",
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
		response = &papi.PhotoCreateCardStoryShootingsResponse{
			Photos: result.Photos, IsGift: result.IsGift, CommonResponse: viber.NewCommonResponse(result.Updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
