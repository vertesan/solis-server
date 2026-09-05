package photo

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CheckExpressionShooting(
	ctx context.Context,
	req *papi.PhotoCheckExpressionShootingRequest,
) (*papi.PhotoCheckExpressionShootingResponse, error) {
	if req.GetPhotoExpressionId() == "" {
		return nil, viber.InvalidArgumentf("photoExpressionId is required")
	}
	if _, err := opmaster.FindPhotoExpression(ctx, req.GetPhotoExpressionId()); err != nil {
		return nil, viber.PhotoMasterLookupError("expression", req.GetPhotoExpressionId(), err)
	}
	suid := viber.GetSuid(ctx).String()
	expressions, err := opuser.FindAllUserPhotoExpressions(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if !unlockedPhotoExpression(expressions, req.GetPhotoExpressionId()) {
		return nil, viber.FailedPreconditionf("photo expression %q is not owned or is locked", req.GetPhotoExpressionId())
	}

	singularSet := req.GetCharacterId() != "" || req.GetCostumeId() != "" || req.GetHairId() != ""
	if singularSet && (req.GetCharacterId() == "" || req.GetCostumeId() == "" || req.GetHairId() == "") {
		return nil, viber.InvalidArgumentf("characterId, costumeId, and hairId must be provided together")
	}
	if err := viber.ValidateOptionalPhotoStyleLists(req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
		return nil, err
	}
	if !singularSet && len(req.GetCharacterIds()) == 0 {
		return nil, viber.InvalidArgumentf("at least one character is required")
	}
	if singularSet {
		if err := viber.ValidateOwnedPhotoStyles(ctx, suid, []string{req.GetCharacterId()}, []string{req.GetCostumeId()}, []string{req.GetHairId()}); err != nil {
			return nil, viber.GameplayError(err)
		}
	}
	if len(req.GetCharacterIds()) > 0 {
		if err := viber.ValidateOwnedPhotoStyles(ctx, suid, req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
			return nil, viber.GameplayError(err)
		}
	}
	return &papi.PhotoCheckExpressionShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
