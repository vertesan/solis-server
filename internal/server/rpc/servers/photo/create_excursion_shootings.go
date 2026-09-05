package photo

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CreateExcursionShootings(
	ctx context.Context,
	req *papi.PhotoCreateExcursionShootingsRequest,
) (*papi.PhotoCreateExcursionShootingsResponse, error) {
	if len(req.GetCreateExcursionShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createExcursionShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateExcursionShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckExcursionShooting(ctx, &papi.PhotoCheckExcursionShootingRequest{
			ExcursionPlaceId: req.GetExcursionPlaceId(), CharacterId: req.GetCharacterId(),
			CostumeId: req.GetCostumeId(), HairId: req.GetHairId(),
		}); err != nil {
			return nil, err
		}
		place, err := opmaster.FindExcursionPlace(ctx, req.GetExcursionPlaceId())
		if err != nil {
			return nil, err
		}
		params := make([]viber.PhotoCreateParam, 0, len(req.GetCreateExcursionShootingParams()))
		for _, param := range req.GetCreateExcursionShootingParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createExcursionShootingParam is required")
			}
			params = append(params, viber.PhotoCreateParam{
				ImageID: param.GetPhotoImageId(), ItemID: param.GetItemId(), FocusCharacterID: req.GetCharacterId(),
			})
		}
		scene := viber.PhotoShootingScene{
			PlaceName: place.GetName(), EventName: place.GetName(), CandidateCharacterIDs: []string{req.GetCharacterId()},
		}
		result, err := viber.CreateUserPhotoShootings(ctx, suid, scene, params, req.GetManualCount())
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoCreateExcursionShootingsResponse{
			Photos: result.Photos, IsGift: result.IsGift, CommonResponse: viber.NewCommonResponse(result.Updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
