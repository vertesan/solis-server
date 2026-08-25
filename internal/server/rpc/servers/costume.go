package servers

import (
	"context"
	"errors"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

type CostumeServer struct {
	papi.UnimplementedCostumeServer
}

func (s *CostumeServer) SetCostume(ctx context.Context, req *papi.CostumeSetRequest) (*papi.CostumeSetResponse, error) {
	if req.GetCostumeId() == "" || req.GetHairId() == "" {
		return nil, viber.InvalidArgumentf("costumeId and hairId are required")
	}
	updated, err := setCharacterCostume(ctx, req.CostumeId, req.HairId, false)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.CostumeSetResponse{CommonResponse: viber.NewCommonResponse(updated)}, nil
}

func (s *CostumeServer) SetLiveCostume(ctx context.Context, req *papi.CostumeLiveSetRequest) (*papi.CostumeLiveSetResponse, error) {
	if req.GetCostumeId() == "" || req.GetHairId() == "" {
		return nil, viber.InvalidArgumentf("costumeId and hairId are required")
	}
	updated, err := setCharacterCostume(ctx, req.CostumeId, req.HairId, true)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.CostumeLiveSetResponse{CommonResponse: viber.NewCommonResponse(updated)}, nil
}

func (s *CostumeServer) CheckCostume(ctx context.Context, req *papi.CostumeCheckRequest) (*papi.CostumeCheckResponse, error) {
	if req.GetCostumeId() == "" {
		return nil, viber.InvalidArgumentf("costumeId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var updated *papi.UpdatedData
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		attemptUpdated := &papi.UpdatedData{}
		costume, err := opuser.FindUserCostume(ctx, suid, req.GetCostumeId())
		if err != nil {
			return nil, err
		}
		if !costume.Checked {
			costume.Checked = true
			if err := opuser.UpsertUserCostume(ctx, suid, costume); err != nil {
				return nil, err
			}
			attemptUpdated.Costumes = append(attemptUpdated.Costumes, costume)
		}
		updated = attemptUpdated
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.CostumeCheckResponse{CommonResponse: viber.NewCommonResponse(updated)}, nil
}

func setCharacterCostume(ctx context.Context, costumeId, hairId string, live bool) (*papi.UpdatedData, error) {
	suid := viber.GetSuid(ctx).String()
	var updated *papi.UpdatedData
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		attemptUpdated := &papi.UpdatedData{}
		masterCostume, err := opmaster.FindCostume(ctx, costumeId)
		if err != nil {
			return nil, err
		}
		masterHair, err := opmaster.FindHair(ctx, hairId)
		if err != nil {
			return nil, err
		}
		if masterCostume.CharacterId == "" || masterCostume.CharacterId != masterHair.CharacterId {
			return nil, viber.FailedPreconditionf("costume and hair belong to different characters")
		}
		if slices.Contains(masterHair.NotWearableCostumeIds, costumeId) ||
			len(masterHair.WearableCostumeIds) > 0 && !slices.Contains(masterHair.WearableCostumeIds, costumeId) {
			return nil, viber.FailedPreconditionf("hair cannot be worn with costume")
		}
		costume, err := opuser.FindUserCostume(ctx, suid, costumeId)
		if err != nil {
			return nil, err
		}
		hair, err := opuser.FindUserHair(ctx, suid, hairId)
		if err != nil {
			return nil, err
		}
		character, err := opuser.FindUserCharacter(ctx, suid, masterCostume.CharacterId)
		if err != nil {
			return nil, err
		}
		if live {
			character.LiveCostumeId, character.LiveHairId = costumeId, hairId
		} else {
			character.FavoriteCostumeId, character.FavoriteHairId = costumeId, hairId
		}
		if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
			return nil, err
		}
		attemptUpdated.Characters = append(attemptUpdated.Characters, character)
		if live && costume.Checked {
			costume.Checked = false
			if err := opuser.UpsertUserCostume(ctx, suid, costume); err != nil {
				return nil, err
			}
			attemptUpdated.Costumes = append(attemptUpdated.Costumes, costume)
		}
		if !live && hair.Checked {
			hair.Checked = false
			if err := opuser.UpsertUserHair(ctx, suid, hair); err != nil {
				return nil, err
			}
			attemptUpdated.Hairs = append(attemptUpdated.Hairs, hair)
		}
		updated = attemptUpdated
		return nil, nil
	})
	return updated, err
}

func (s *CostumeServer) CheckBulk(ctx context.Context, req *papi.CostumeCheckBulkRequest) (*papi.CostumeCheckBulkResponse, error) {
	if len(req.GetCostumeIds()) == 0 && len(req.GetHairIds()) == 0 {
		return nil, viber.InvalidArgumentf("at least one costumeId or hairId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var updated *papi.UpdatedData
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		attemptUpdated := &papi.UpdatedData{}
		seenCostumes := make(map[string]struct{}, len(req.CostumeIds))
		for _, id := range req.CostumeIds {
			if id == "" {
				return nil, viber.InvalidArgumentf("costumeId is required")
			}
			if _, exists := seenCostumes[id]; exists {
				continue
			}
			seenCostumes[id] = struct{}{}
			costume, err := opuser.FindUserCostume(ctx, suid, id)
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if costume.Checked {
				continue
			}
			costume.Checked = true
			if err := opuser.UpsertUserCostume(ctx, suid, costume); err != nil {
				return nil, err
			}
			attemptUpdated.Costumes = append(attemptUpdated.Costumes, costume)
		}
		seenHairs := make(map[string]struct{}, len(req.HairIds))
		for _, id := range req.HairIds {
			if id == "" {
				return nil, viber.InvalidArgumentf("hairId is required")
			}
			if _, exists := seenHairs[id]; exists {
				continue
			}
			seenHairs[id] = struct{}{}
			hair, err := opuser.FindUserHair(ctx, suid, id)
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if hair.Checked {
				continue
			}
			hair.Checked = true
			if err := opuser.UpsertUserHair(ctx, suid, hair); err != nil {
				return nil, err
			}
			attemptUpdated.Hairs = append(attemptUpdated.Hairs, hair)
		}
		updated = attemptUpdated
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.CostumeCheckBulkResponse{CommonResponse: viber.NewCommonResponse(updated)}, nil
}
