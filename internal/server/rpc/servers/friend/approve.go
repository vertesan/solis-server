package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) Approve(_ context.Context, _ *papi.FriendApproveRequest) (*papi.FriendApproveResponse, error) {
	// There are no incoming offers to approve; do not report a phantom friend.
	return &papi.FriendApproveResponse{
		IsOfferDeleted: true,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
