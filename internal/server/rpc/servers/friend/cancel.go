package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) Cancel(_ context.Context, _ *papi.FriendCancelRequest) (*papi.FriendCancelResponse, error) {
	return &papi.FriendCancelResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
