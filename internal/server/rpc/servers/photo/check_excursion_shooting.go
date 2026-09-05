package photo

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CheckExcursionShooting(
	ctx context.Context,
	req *papi.PhotoCheckExcursionShootingRequest,
) (*papi.PhotoCheckExcursionShootingResponse, error) {
	if req.GetExcursionPlaceId() == "" || req.GetCharacterId() == "" || req.GetCostumeId() == "" || req.GetHairId() == "" {
		return nil, viber.InvalidArgumentf("excursionPlaceId, characterId, costumeId, and hairId are required")
	}
	if _, err := opmaster.FindExcursionPlace(ctx, req.GetExcursionPlaceId()); err != nil {
		return nil, viber.PhotoMasterLookupError("excursion place", req.GetExcursionPlaceId(), err)
	}
	suid := viber.GetSuid(ctx).String()
	if err := viber.ValidateOwnedPhotoStyles(ctx, suid, []string{req.GetCharacterId()}, []string{req.GetCostumeId()}, []string{req.GetHairId()}); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.PhotoCheckExcursionShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
