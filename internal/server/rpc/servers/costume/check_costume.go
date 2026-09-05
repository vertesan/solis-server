package costume

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

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
