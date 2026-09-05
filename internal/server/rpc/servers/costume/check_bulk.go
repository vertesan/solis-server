package costume

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

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
