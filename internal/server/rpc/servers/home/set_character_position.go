package home

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *HomeServer) SetCharacterPosition(
	ctx context.Context,
	req *papi.HomeSetCharacterPositionRequest,
) (*papi.HomeSetCharacterPositionResponse, error) {
	positions, err := homePositionsFromRequest(req)
	if err != nil {
		return nil, err
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.HomeSetCharacterPositionResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		var randomCharacterIDs []string
		for _, position := range positions {
			if !position.GetIsCharacterRandom() {
				continue
			}
			characters, err := opuser.FindAllUserCharacters(ctx, suid)
			if err != nil {
				return nil, err
			}
			for _, character := range characters {
				if character.GetCharacterId() != "" {
					randomCharacterIDs = append(randomCharacterIDs, character.GetCharacterId())
				}
			}
			break
		}
		for _, position := range positions {
			if position.GetIsCharacterRandom() {
				if len(randomCharacterIDs) == 0 {
					return nil, status.Error(codes.FailedPrecondition, "no owned character is available for a random home position")
				}
				index, err := viber.CryptoRandomIndex(len(randomCharacterIDs))
				if err != nil {
					return nil, err
				}
				position.CharacterId = randomCharacterIDs[index]
				continue
			}
			if _, err := opuser.FindUserCharacter(ctx, suid, position.GetCharacterId()); err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, status.Errorf(codes.PermissionDenied, "character %q is not owned", position.GetCharacterId())
				}
				return nil, err
			}
		}
		if err := opuser.UpsertManyUserHomePositions(ctx, suid, positions); err != nil {
			return nil, err
		}
		response = &papi.HomeSetCharacterPositionResponse{
			CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{HomePositions: positions}),
		}
		return nil, nil
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, status.Error(codes.FailedPrecondition, "home positions changed concurrently")
		}
		return nil, err
	}
	return response, nil
}

func homePositionsFromRequest(req *papi.HomeSetCharacterPositionRequest) ([]*ptransaction.UserHomePosition, error) {
	if req == nil || len(req.GetPositions()) != 3 {
		return nil, status.Error(codes.InvalidArgument, "home, training, and activity positions are required")
	}
	requiredTypes := map[penum.HomePositionType]bool{
		penum.HomePositionType_HomePositionType_Home:     false,
		penum.HomePositionType_HomePositionType_Training: false,
		penum.HomePositionType_HomePositionType_Activity: false,
	}
	positions := make([]*ptransaction.UserHomePosition, 0, len(req.GetPositions()))
	for _, requested := range req.GetPositions() {
		if requested == nil {
			return nil, status.Error(codes.InvalidArgument, "position must not be nil")
		}
		positionType := requested.GetType()
		seen, valid := requiredTypes[positionType]
		if !valid {
			return nil, status.Error(codes.InvalidArgument, "unknown home position type")
		}
		if seen {
			return nil, status.Error(codes.InvalidArgument, "home position types must be unique")
		}
		requiredTypes[positionType] = true
		if requested.GetIsCharacterRandom() && requested.GetCharacterId() != "" {
			return nil, status.Error(codes.InvalidArgument, "random position must not specify a character")
		}
		if !requested.GetIsCharacterRandom() && requested.GetCharacterId() == "" {
			return nil, status.Error(codes.InvalidArgument, "fixed position requires a character")
		}
		positions = append(positions, &ptransaction.UserHomePosition{
			HomePositionType:  positionType,
			CharacterId:       requested.GetCharacterId(),
			IsCharacterRandom: requested.GetIsCharacterRandom(),
			IsCostumeRandom:   requested.GetIsCostumeRandom(),
		})
	}
	return positions, nil
}
