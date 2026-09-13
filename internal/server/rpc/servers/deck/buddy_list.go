package deck

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DeckServer) BuddyList(context.Context, *papi.Empty) (*papi.DeckBuddyListResponse, error) {
	return &papi.DeckBuddyListResponse{
		FriendBuddyUserCardInfos:    []*papi.DeckBuddyUserCardInfo{},
		NotFriendBuddyUserCardInfos: []*papi.DeckBuddyUserCardInfo{},
		CommonResponse:              viber.NewCommonResponse(nil),
	}, nil
}
