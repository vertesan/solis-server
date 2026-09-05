package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ShowcaseServer) CheckShooting(ctx context.Context, req *papi.ShowcaseCheckShootingRequest) (*papi.ShowcaseCheckShootingResponse, error) {
	if req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("number is required")
	}
	suid := viber.GetSuid(ctx).String()
	if _, _, err := findSavedShowcase(ctx, suid, req.GetNumber()); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ShowcaseCheckShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
