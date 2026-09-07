package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) Delete(_ context.Context, _ *papi.FriendDeleteRequest) (*papi.FriendDeleteResponse, error) {
	return &papi.FriendDeleteResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
