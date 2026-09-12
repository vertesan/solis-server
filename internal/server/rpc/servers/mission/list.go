package mission

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *MissionServer) List(ctx context.Context, req *papi.Empty) (*papi.MissionListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.MissionListResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		catalog, err := viber.LoadActiveMissionCatalog(ctx, suid, time.Now().UnixMilli())
		if err != nil {
			return nil, err
		}
		if err := catalog.Reconcile(ctx, suid, time.Now().UnixMilli(), updated); err != nil {
			return nil, err
		}
		rows, err := missionRows(ctx, suid, catalog, "")
		if err != nil {
			return nil, err
		}
		response = &papi.MissionListResponse{UserMissions: rows, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
