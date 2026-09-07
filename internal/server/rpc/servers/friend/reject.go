package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) Reject(_ context.Context, _ *papi.FriendRejectRequest) (*papi.FriendRejectResponse, error) {
	return &papi.FriendRejectResponse{
		IsOfferDeleted: true,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
