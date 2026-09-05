package photopanorama

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoPanoramaServer) CheckShooting(
	ctx context.Context,
	req *papi.PhotoPanoramaCheckShootingRequest,
) (*papi.PhotoPanoramaCheckShootingResponse, error) {
	if _, err := checkPhotoPanoramaShooting(ctx, req); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.PhotoPanoramaCheckShootingResponse{
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
