package photo

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CheckCardStoryShooting(
	ctx context.Context,
	req *papi.PhotoCheckCardStoryShootingRequest,
) (*papi.PhotoCheckCardStoryShootingResponse, error) {
	if req.GetCardId() == "" || req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("cardId and positive number are required")
	}
	card, err := opmaster.FindCard(ctx, req.GetCardId())
	if err != nil {
		return nil, viber.PhotoMasterLookupError("card", req.GetCardId(), err)
	}
	if int(req.GetNumber()) > len(card.GetStories()) {
		return nil, viber.InvalidArgumentf("card story number is out of range")
	}
	suid := viber.GetSuid(ctx).String()
	if _, err := opuser.FindUserCard(ctx, suid, req.GetCardId()); err != nil {
		return nil, viber.PhotoOwnershipError("card", req.GetCardId(), err)
	}
	storyID := card.GetStories()[req.GetNumber()-1].GetStoryId()
	story, err := opuser.FindUserStory(ctx, suid, storyID)
	if err != nil {
		return nil, viber.PhotoOwnershipError("card story", storyID, err)
	}
	if story.GetIsInvalid() || story.GetStatusType() == penum.StoryStatusType_StoryStatusType_Unknown {
		return nil, viber.FailedPreconditionf("card story %q is unavailable", storyID)
	}
	return &papi.PhotoCheckCardStoryShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
