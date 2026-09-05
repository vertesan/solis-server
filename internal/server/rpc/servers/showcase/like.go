package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ShowcaseServer) Like(ctx context.Context, req *papi.ShowcaseLikeRequest) (*papi.ShowcaseLikeResponse, error) {
	if req.GetUserId() == "" || req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("userId and number are required")
	}
	return &papi.ShowcaseLikeResponse{
		Rewards:        []*ptransaction.Reward{},
		IsAlreadyLike:  true,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
