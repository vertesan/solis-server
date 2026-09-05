package costume

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CostumeServer) SetLiveCostume(ctx context.Context, req *papi.CostumeLiveSetRequest) (*papi.CostumeLiveSetResponse, error) {
	if req.GetCostumeId() == "" || req.GetHairId() == "" {
		return nil, viber.InvalidArgumentf("costumeId and hairId are required")
	}
	updated, err := setCharacterCostume(ctx, req.CostumeId, req.HairId, true)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.CostumeLiveSetResponse{CommonResponse: viber.NewCommonResponse(updated)}, nil
}
