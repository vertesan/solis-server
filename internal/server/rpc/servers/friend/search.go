package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) Search(_ context.Context, _ *papi.Empty) (*papi.FriendSearchResponse, error) {
	return &papi.FriendSearchResponse{
		Profiles:       []*papi.ProfileInfo{},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
