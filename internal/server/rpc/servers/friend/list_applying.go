package friend

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *FriendServer) ListApplying(_ context.Context, _ *papi.Empty) (*papi.FriendListApplyingResponse, error) {
	return &papi.FriendListApplyingResponse{
		Profiles:       []*papi.ProfileInfo{},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
