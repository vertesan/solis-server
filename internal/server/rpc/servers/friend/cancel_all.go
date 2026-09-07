package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) CancelAll(_ context.Context, _ *papi.Empty) (*papi.FriendCancelAllResponse, error) {
	return &papi.FriendCancelAllResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
