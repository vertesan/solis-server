package photo

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CreateShootings(
	ctx context.Context,
	req *papi.PhotoCreateShootingsRequest,
) (*papi.PhotoCreateShootingsResponse, error) {
	if len(req.GetCreateShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckShooting(ctx, &papi.PhotoCheckShootingRequest{
			ActionType:      req.GetActionType(),
			PhotoActivityId: req.GetPhotoActivityId(),
			PhotoMusicId:    req.GetPhotoMusicId(),
			PhotoStageId:    req.GetPhotoStageId(),
			CharacterIds:    req.GetCharacterIds(),
			CostumeIds:      req.GetCostumeIds(),
			HairIds:         req.GetHairIds(),
		}); err != nil {
			return nil, err
		}
		scene, err := normalPhotoScene(ctx, req)
		if err != nil {
			return nil, err
		}
		params := make([]viber.PhotoCreateParam, 0, len(req.GetCreateShootingParams()))
		for _, param := range req.GetCreateShootingParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createShootingParam is required")
			}
			params = append(params, viber.PhotoCreateParam{
				ImageID:          param.GetPhotoImageId(),
				ItemID:           param.GetItemId(),
				FocusCharacterID: param.GetMainCharacterId(),
			})
		}
		result, err := viber.CreateUserPhotoShootings(ctx, suid, scene, params, req.GetManualCount())
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoCreateShootingsResponse{
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

func normalPhotoScene(ctx context.Context, req *papi.PhotoCreateShootingsRequest) (viber.PhotoShootingScene, error) {
	scene := viber.PhotoShootingScene{CandidateCharacterIDs: slices.Clone(req.GetCharacterIds())}
	switch req.GetActionType() {
	case penum.PhotoShootingActionType_PhotoShootingActionType_Quest:
		stage, err := opmaster.FindPhotoQuestStage(ctx, req.GetPhotoStageId())
		if err != nil {
			return scene, err
		}
		scene.PlaceName = stage.GetName()
		scene.EventName = "ライブ"
		scene.MusicID = req.GetPhotoMusicId()
	case penum.PhotoShootingActionType_PhotoShootingActionType_ActivityPromotion,
		penum.PhotoShootingActionType_PhotoShootingActionType_ActivityRefresh:
		activity, err := opmaster.FindPhotoActivity(ctx, req.GetPhotoActivityId())
		if err != nil {
			return scene, err
		}
		scene.PlaceName = activity.GetName()
		scene.EventName = activity.GetName()
	default:
		return scene, viber.InvalidArgumentf("unsupported photo shooting action type")
	}
	return scene, nil
}
