package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) Apply(_ context.Context, _ *papi.FriendApplyRequest) (*papi.FriendApplyResponse, error) {
	// Other accounts are unavailable in the private server, even for a known ID.
	return &papi.FriendApplyResponse{
		IsAccountDeleted: true,
		CommonResponse:   viber.NewCommonResponse(nil),
	}, nil
}
