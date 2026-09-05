package photo

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CheckShooting(
	ctx context.Context,
	req *papi.PhotoCheckShootingRequest,
) (*papi.PhotoCheckShootingResponse, error) {
	if err := viber.ValidatePhotoStyleLists(req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()

	switch req.GetActionType() {
	case penum.PhotoShootingActionType_PhotoShootingActionType_ActivityPromotion,
		penum.PhotoShootingActionType_PhotoShootingActionType_ActivityRefresh:
		if req.GetPhotoActivityId() == "" {
			return nil, viber.InvalidArgumentf("photoActivityId is required")
		}
		activity, err := opmaster.FindPhotoActivity(ctx, req.GetPhotoActivityId())
		if err != nil {
			return nil, viber.PhotoMasterLookupError("activity", req.GetPhotoActivityId(), err)
		}
		if activity.GetType() != req.GetActionType() {
			return nil, viber.InvalidArgumentf("photo activity %q has a different action type", req.GetPhotoActivityId())
		}
		userActivity, err := opuser.FindUserPhotoActivity(ctx, suid, req.GetPhotoActivityId())
		if err != nil {
			return nil, viber.PhotoOwnershipError("photo activity", req.GetPhotoActivityId(), err)
		}
		if !userActivity.GetIsUnlocked() {
			return nil, viber.FailedPreconditionf("photo activity %q is locked", req.GetPhotoActivityId())
		}
	case penum.PhotoShootingActionType_PhotoShootingActionType_Quest:
		if req.GetPhotoMusicId() == "" || req.GetPhotoStageId() == "" {
			return nil, viber.InvalidArgumentf("photoMusicId and photoStageId are required")
		}
		if _, err := opmaster.FindPhotoQuestMusic(ctx, req.GetPhotoMusicId()); err != nil {
			return nil, viber.PhotoMasterLookupError("music", req.GetPhotoMusicId(), err)
		}
		if _, err := opmaster.FindPhotoQuestStage(ctx, req.GetPhotoStageId()); err != nil {
			return nil, viber.PhotoMasterLookupError("stage", req.GetPhotoStageId(), err)
		}
		music, err := opuser.FindUserPhotoMusic(ctx, suid, req.GetPhotoMusicId())
		if err != nil {
			return nil, viber.PhotoOwnershipError("photo music", req.GetPhotoMusicId(), err)
		}
		if !music.GetIsUnlocked() {
			return nil, viber.FailedPreconditionf("photo music %q is locked", req.GetPhotoMusicId())
		}
		stages, err := opuser.FindAllUserPhotoStages(ctx, suid)
		if err != nil {
			return nil, err
		}
		if !unlockedPhotoStage(stages, req.GetPhotoStageId()) {
			return nil, viber.FailedPreconditionf("photo stage %q is not owned or is locked", req.GetPhotoStageId())
		}
	default:
		return nil, viber.InvalidArgumentf("unsupported photo shooting action type")
	}

	if err := viber.ValidateOwnedPhotoStyles(ctx, suid, req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.PhotoCheckShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
