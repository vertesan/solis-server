package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ShowcaseServer) Display(ctx context.Context, req *papi.ShowcaseDisplayRequest) (*papi.ShowcaseDisplayResponse, error) {
	if req.GetUserId() == "" || req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("userId and number are required")
	}
	return &papi.ShowcaseDisplayResponse{
		IsViewable:     false,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
