package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ShowcaseServer) Get(ctx context.Context, req *papi.ShowcaseGetRequest) (*papi.ShowcaseGetResponse, error) {
	if req.GetUserId() == "" {
		return nil, viber.InvalidArgumentf("userId is required")
	}
	return &papi.ShowcaseGetResponse{
		ShowcaseInfos:  []*papi.ShowcaseInfo{},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
