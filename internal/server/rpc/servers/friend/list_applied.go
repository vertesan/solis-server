package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) ListApplied(_ context.Context, _ *papi.Empty) (*papi.FriendListAppliedResponse, error) {
	return &papi.FriendListAppliedResponse{
		Profiles:       []*papi.ProfileInfo{},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
