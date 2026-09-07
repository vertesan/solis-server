package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) List(_ context.Context, _ *papi.Empty) (*papi.FriendListResponse, error) {
	return &papi.FriendListResponse{
		Profiles:       []*papi.ProfileInfo{},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
