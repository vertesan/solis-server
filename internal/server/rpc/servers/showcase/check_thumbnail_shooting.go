package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ShowcaseServer) CheckThumbnailShooting(ctx context.Context, req *papi.ShowcaseCheckThumbnailShootingRequest) (*papi.ShowcaseCheckThumbnailShootingResponse, error) {
	if req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("number is required")
	}
	suid := viber.GetSuid(ctx).String()
	if err := requireUnlockedShowcaseNumber(ctx, suid, req.GetNumber()); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ShowcaseCheckThumbnailShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
