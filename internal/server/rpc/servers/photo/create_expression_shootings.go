package photo

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) CreateExpressionShootings(
	ctx context.Context,
	req *papi.PhotoCreateExpressionShootingsRequest,
) (*papi.PhotoCreateExpressionShootingsResponse, error) {
	if len(req.GetCreateExpressionShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createExpressionShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateExpressionShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckExpressionShooting(ctx, &papi.PhotoCheckExpressionShootingRequest{
			PhotoExpressionId: req.GetPhotoExpressionId(),
			CharacterId:       req.GetCharacterId(),
			CostumeId:         req.GetCostumeId(),
			HairId:            req.GetHairId(),
			CharacterIds:      req.GetCharacterIds(),
			CostumeIds:        req.GetCostumeIds(),
			HairIds:           req.GetHairIds(),
		}); err != nil {
			return nil, err
		}
		master, err := opmaster.FindPhotoExpression(ctx, req.GetPhotoExpressionId())
		if err != nil {
			return nil, err
		}
		stage, err := opmaster.FindPhotoStage(ctx, master.GetStageId())
		if err != nil {
			return nil, err
		}
		positions, err := opmaster.FindPhotoExpressionStagePositions(ctx, master.GetStageId())
		if err != nil {
			return nil, err
		}
		validPositions := make(map[int32]struct{}, len(positions))
		for _, position := range positions {
			validPositions[position.GetNumber()] = struct{}{}
		}
		candidateIDs := req.GetCharacterIds()
		if len(candidateIDs) == 0 {
			candidateIDs = []string{req.GetCharacterId()}
		}
		params := make([]viber.PhotoCreateParam, 0, len(req.GetCreateExpressionShootingParams()))
		specifiedPoseCounts := make(map[string]int64)
		specifiedCharacterCounts := make(map[string]int64)
		for _, param := range req.GetCreateExpressionShootingParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createExpressionShootingParam is required")
			}
			if _, exists := validPositions[param.GetStagePositionNumber()]; !exists {
				return nil, viber.InvalidArgumentf("unknown expression stage position")
			}
			if err := validateExpressionPhotoParam(ctx, suid, param); err != nil {
				return nil, err
			}
			if param.GetPhotoPoseId() != "" {
				specifiedPoseCounts[param.GetPhotoPoseId()]++
			}
			if param.GetMainCharacterId() != "" {
				specifiedCharacterCounts[param.GetMainCharacterId()]++
			}
			params = append(params, viber.PhotoCreateParam{
				ImageID: param.GetPhotoImageId(), ItemID: param.GetItemId(), FocusCharacterID: param.GetMainCharacterId(),
			})
		}
		scene := viber.PhotoShootingScene{
			PlaceName:                stage.GetName(),
			EventName:                master.GetName(),
			CandidateCharacterIDs:    slices.Clone(candidateIDs),
			ExpressionShooting:       true,
			SpecifiedPoseCounts:      specifiedPoseCounts,
			SpecifiedCharacterCounts: specifiedCharacterCounts,
		}
		result, err := viber.CreateUserPhotoShootings(ctx, suid, scene, params, req.GetManualCount())
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoCreateExpressionShootingsResponse{
			Photos: result.Photos, IsGift: result.IsGift, CommonResponse: viber.NewCommonResponse(result.Updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func validateExpressionPhotoParam(
	ctx context.Context,
	suid string,
	param *papi.PhotoCreateExpressionShootingParam,
) error {
	if param.GetPhotoImageId() == "" || param.GetItemId() == "" || param.GetMainCharacterId() == "" {
		return viber.InvalidArgumentf("photoImageId, itemId, and mainCharacterId are required")
	}
	if len(param.GetCharacterIds()) > 0 {
		if len(param.GetPhotoPoseIds()) != len(param.GetCharacterIds()) ||
			len(param.GetPhotoFacialIds()) != len(param.GetCharacterIds()) ||
			len(param.GetPhotoLookingTypes()) != len(param.GetCharacterIds()) {
			return viber.InvalidArgumentf("expression character option lists must have equal lengths")
		}
	}
	poseIDs := append(slices.Clone(param.GetPhotoPoseIds()), param.GetPhotoPoseId())
	for _, poseID := range poseIDs {
		if poseID == "" {
			continue
		}
		if _, err := opmaster.FindPhotoPose(ctx, poseID); err != nil {
			return viber.PhotoMasterLookupError("photo pose", poseID, err)
		}
		if _, err := opuser.FindUserPhotoPose(ctx, suid, poseID); err != nil {
			return viber.PhotoOwnershipError("photo pose", poseID, err)
		}
	}
	facialIDs := append(slices.Clone(param.GetPhotoFacialIds()), param.GetPhotoFacialId())
	for _, facialID := range facialIDs {
		if facialID == "" {
			continue
		}
		if _, err := opmaster.FindPhotoFacial(ctx, facialID); err != nil {
			return viber.PhotoMasterLookupError("photo facial", facialID, err)
		}
	}
	if !slices.Contains(param.GetCharacterIds(), param.GetMainCharacterId()) && len(param.GetCharacterIds()) > 0 {
		return viber.FailedPreconditionf("main character is absent from expression characters")
	}
	return nil
}
