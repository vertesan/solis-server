package costume

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CostumeServer) SetCostume(ctx context.Context, req *papi.CostumeSetRequest) (*papi.CostumeSetResponse, error) {
	if req.GetCostumeId() == "" || req.GetHairId() == "" {
		return nil, viber.InvalidArgumentf("costumeId and hairId are required")
	}
	updated, err := setCharacterCostume(ctx, req.CostumeId, req.HairId, false)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.CostumeSetResponse{CommonResponse: viber.NewCommonResponse(updated)}, nil
}
