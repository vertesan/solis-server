package servers

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

type HairServer struct {
	papi.UnimplementedHairServer
}

func (s *HairServer) CheckHair(ctx context.Context, req *papi.HairCheckRequest) (*papi.HairCheckResponse, error) {
	if req.GetHairId() == "" {
		return nil, viber.InvalidArgumentf("hairId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var updated *papi.UpdatedData
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		attemptUpdated := &papi.UpdatedData{}
		hair, err := opuser.FindUserHair(ctx, suid, req.GetHairId())
		if err != nil {
			return nil, err
		}
		if !hair.Checked {
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
	return &papi.HairCheckResponse{CommonResponse: viber.NewCommonResponse(updated)}, nil
}
