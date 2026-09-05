package company

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CompanyServer) BulkReceiveMission(
	ctx context.Context,
	req *papi.CompanyBulkReceiveMissionRequest,
) (*papi.CompanyBulkReceiveMissionResponse, error) {
	if err := validateCompanyMissionIDs(req.GetMissionIds()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyBulkReceiveMissionResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		results, _, _, err := receiveCompanyMissions(ctx, suid, req.GetMissionIds(), false, updated)
		if err != nil {
			return nil, err
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyBulkReceiveMissionResponse{
			Results:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
