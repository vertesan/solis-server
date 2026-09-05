package showcase

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ShowcaseServer) CreatePhoto(ctx context.Context, req *papi.ShowcaseCreatePhotoRequest) (*papi.ShowcaseCreatePhotoResponse, error) {
	if req.GetNumber() <= 0 || len(req.GetCreatePhotoParams()) == 0 {
		return nil, viber.InvalidArgumentf("number and at least one createPhotoParam are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseCreatePhotoResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		showcase, layout, err := findSavedShowcase(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		candidates, err := showcasePhotoCharacters(ctx, layout)
		if err != nil {
			return nil, err
		}
		params := make([]viber.PhotoCreateParam, 0, len(req.GetCreatePhotoParams()))
		for _, param := range req.GetCreatePhotoParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createPhotoParam is required")
			}
			if param.GetAssetId() == "" || param.GetItemId() == "" {
				return nil, viber.InvalidArgumentf("assetId and itemId are required")
			}
			film, err := opmaster.FindItem(ctx, param.GetItemId())
			if err != nil {
				return nil, err
			}
			if film.GetIsPhotoShootingCharacterOnly() {
				return nil, status.Error(codes.FailedPrecondition, "character-only shooting film cannot be used for showcase shooting")
			}
			params = append(params, viber.PhotoCreateParam{
				ImageID:          param.GetAssetId(),
				ItemID:           param.GetItemId(),
				FocusCharacterID: param.GetMainCharacterId(),
			})
		}
		placeName := showcase.GetName()
		if placeName == "" {
			placeName = showcaseDefaultName(req.GetNumber())
		}
		result, err := viber.CreateUserPhotoShootings(ctx, suid, viber.PhotoShootingScene{
			PlaceName:                   placeName,
			EventName:                   showcaseEventName,
			PhotoNamePrefix:             showcaseEventName,
			CandidateCharacterIDs:       candidates,
			ShowcaseShooting:            true,
			ReliabilityOncePerCharacter: true,
		}, params, req.GetManualCount())
		if err != nil {
			return nil, err
		}
		response = &papi.ShowcaseCreatePhotoResponse{
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

func showcasePhotoCharacters(ctx context.Context, layout *papi.ShowcaseLayoutInfo) ([]string, error) {
	characters := make([]string, 0)
	seenToys := make(map[string]struct{})
	for _, asset := range layout.GetShowcaseToyAssetInfos() {
		if asset == nil {
			continue
		}
		if _, seen := seenToys[asset.GetShowcaseToyId()]; seen {
			continue
		}
		seenToys[asset.GetShowcaseToyId()] = struct{}{}
		toy, err := opmaster.FindShowcaseToy(ctx, asset.GetShowcaseToyId())
		if err != nil {
			return nil, err
		}
		for _, characterID := range toy.GetPhotoShootingCharacterIds() {
			if characterID != "" && !slices.Contains(characters, characterID) {
				characters = append(characters, characterID)
			}
		}
	}
	return characters, nil
}
