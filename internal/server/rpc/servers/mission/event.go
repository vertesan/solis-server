package mission

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *MissionServer) Event(ctx context.Context, req *papi.MissionEventRequest) (*papi.MissionEventResponse, error) {
	if req.GetEventMissionId() == "" {
		return nil, viber.InvalidArgumentf("eventMissionId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.MissionEventResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		now := time.Now().UnixMilli()
		catalog, err := viber.LoadActiveMissionCatalog(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		id := req.GetEventMissionId()
		if catalog.Events[id] == nil {
			return nil, viber.FailedPreconditionf("event mission %q is not available", id)
		}
		if err := catalog.Reconcile(ctx, suid, now, updated); err != nil {
			return nil, err
		}
		rows, err := missionRows(ctx, suid, catalog, id)
		if err != nil {
			return nil, err
		}
		response = &papi.MissionEventResponse{Info: &papi.EventMissionInfo{Id: id, Unlocked: catalog.Unlocked[id], UserMissions: rows}, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
