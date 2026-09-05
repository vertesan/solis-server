package excursion

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ExcursionServer) List(ctx context.Context, req *papi.ExcursionListRequest) (*papi.ExcursionListResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "characterId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ExcursionListResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := requireExcursionUnlocked(ctx, suid); err != nil {
			return nil, err
		}
		if _, err := opuser.FindUserCharacter(ctx, suid, req.GetCharacterId()); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.PermissionDenied, "character is not owned")
			}
			return nil, err
		}
		settings, err := opmaster.FindExcursionPlaceSettingsByCharacter(ctx, req.GetCharacterId())
		if err != nil {
			return nil, err
		}
		placeIDs := make([]string, 0, len(settings))
		seen := make(map[string]struct{}, len(settings))
		for _, setting := range settings {
			placeID := setting.GetExcursionPlaceId()
			if placeID == "" {
				continue
			}
			if _, exists := seen[placeID]; exists {
				continue
			}
			if _, err := opmaster.FindExcursionPlace(ctx, placeID); err != nil {
				return nil, err
			}
			seen[placeID] = struct{}{}
			placeIDs = append(placeIDs, placeID)
		}
		if len(placeIDs) == 0 {
			return nil, status.Error(codes.NotFound, "no excursion places are available")
		}
		if err := shuffleStrings(placeIDs); err != nil {
			return nil, err
		}
		if len(placeIDs) > excursionOfferCount {
			placeIDs = placeIDs[:excursionOfferCount]
		}

		state, err := findExcursionStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		if state.ActiveCharacter != "" {
			return nil, status.Error(codes.FailedPrecondition, "an excursion is already active")
		}
		state.OfferedCharacter = req.GetCharacterId()
		state.OfferedPlaceIds = append([]string(nil), placeIDs...)
		if err := opuser.UpsertUserExcursionState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.ExcursionListResponse{
			ExcursionPlaceIds: append([]string(nil), placeIDs...),
			CommonResponse:    viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

const excursionOfferCount = 2

func shuffleStrings(values []string) error {
	for index := len(values) - 1; index > 0; index-- {
		selected, err := viber.CryptoRandomIndex(index + 1)
		if err != nil {
			return err
		}
		values[index], values[selected] = values[selected], values[index]
	}
	return nil
}
