package profile

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) ClearTwitterMission(ctx context.Context, _ *papi.Empty) (*papi.ProfileClearTwitterMissionResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ProfileClearTwitterMissionResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		missions, err := viber.ReachUserMissions(ctx, suid, penum.MissionType_MissionType_PostProfileToTwitter, 1)
		if err != nil {
			return nil, err
		}
		response = &papi.ProfileClearTwitterMissionResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{Missions: missions})}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
