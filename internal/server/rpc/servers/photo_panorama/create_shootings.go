package photopanorama

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoPanoramaServer) CreateShootings(
	ctx context.Context,
	req *papi.PhotoPanoramaCreateShootingsRequest,
) (*papi.PhotoPanoramaCreateShootingsResponse, error) {
	if len(req.GetCreateShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createShootingParam is required")
	}
	if len(req.GetCameraTypes()) != len(req.GetCreateShootingParams()) {
		return nil, viber.InvalidArgumentf("cameraTypes and createShootingParams must have equal lengths")
	}
	for _, cameraType := range req.GetCameraTypes() {
		if !validPhotoPanoramaCameraType(cameraType) {
			return nil, viber.InvalidArgumentf("unsupported panorama camera type %d", cameraType)
		}
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoPanoramaCreateShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		resolved, err := checkPhotoPanoramaShooting(ctx, &papi.PhotoPanoramaCheckShootingRequest{
			PhotoMusicId: req.GetPhotoMusicId(),
			CharacterIds: req.GetCharacterIds(),
			CostumeIds:   req.GetCostumeIds(),
			HairIds:      req.GetHairIds(),
		})
		if err != nil {
			return nil, err
		}
		if req.GetStartCharacterId() != "" && !slices.Contains(req.GetCharacterIds(), req.GetStartCharacterId()) {
			return nil, viber.InvalidArgumentf("startCharacterId must be one of characterIds")
		}
		if !photoPanoramaCameraPositionExists(resolved.stage, req.GetStartCameraPositionIndex()) {
			return nil, viber.InvalidArgumentf(
				"stage %q has no panorama camera position %d",
				resolved.stage.GetId(), req.GetStartCameraPositionIndex(),
			)
		}
		for _, param := range req.GetCreateShootingParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createShootingParam is required")
			}
			if param.GetMainCharacterId() != "" && !slices.Contains(req.GetCharacterIds(), param.GetMainCharacterId()) {
				return nil, viber.InvalidArgumentf("mainCharacterId must be one of characterIds")
			}
		}

		result, err := viber.CreateUserPhotoShootings(
			ctx,
			suid,
			viber.PhotoShootingScene{
				PlaceName:             resolved.stage.GetName(),
				EventName:             photoPanoramaEventName,
				CandidateCharacterIDs: slices.Clone(req.GetCharacterIds()),
				PanoramaShooting:      true,
				MusicID:               req.GetPhotoMusicId(),
			},
			viber.CommonPhotoParams(req.GetCreateShootingParams()),
			req.GetManualCount(),
		)
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoPanoramaCreateShootingsResponse{
			Photos:         result.Photos,
			IsGift:         result.IsGift,
			CommonResponse: viber.NewCommonResponse(result.Updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func validPhotoPanoramaCameraType(value penum.PanoramaCameraType) bool {
	switch value {
	case penum.PanoramaCameraType_PanoramaCameraType_StageCamera,
		penum.PanoramaCameraType_PanoramaCameraType_DollyCamera,
		penum.PanoramaCameraType_PanoramaCameraType_OshiCamera,
		penum.PanoramaCameraType_PanoramaCameraType_DroneCamera,
		penum.PanoramaCameraType_PanoramaCameraType_AutoCamera:
		return true
	default:
		return false
	}
}

func photoPanoramaCameraPositionExists(stage *pmaster.Stage, index int32) bool {
	for _, position := range stage.GetCameraPositions() {
		if position.GetIndex() == index {
			return true
		}
	}
	return false
}
