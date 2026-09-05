package telephone

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *TelephoneServer) Finishes(
	ctx context.Context,
	req *papi.TelephoneFinishesRequest,
) (*papi.TelephoneFinishesResponse, error) {
	if len(req.GetTelephoneIds()) == 0 {
		return nil, viber.InvalidArgumentf("at least one telephoneId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.TelephoneFinishesResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		seen := make(map[string]struct{}, len(req.GetTelephoneIds()))
		for _, telephoneID := range req.GetTelephoneIds() {
			if telephoneID == "" {
				return nil, viber.InvalidArgumentf("telephoneId must not be empty")
			}
			if _, duplicate := seen[telephoneID]; duplicate {
				continue
			}
			seen[telephoneID] = struct{}{}
			if _, err := opmaster.FindTelephone(ctx, telephoneID); err != nil {
				return nil, err
			}
			telephone, err := opuser.FindUserTelephone(ctx, suid, telephoneID)
			if err != nil {
				return nil, err
			}
			if telephone.GetTelephoneStatusType() == penum.TelephoneStatusType_TelephoneStatusType_Finished {
				continue
			}
			if telephone.GetTelephoneStatusType() != penum.TelephoneStatusType_TelephoneStatusType_Received {
				return nil, status.Errorf(codes.FailedPrecondition, "telephone %q has not been received", telephoneID)
			}
			telephone.TelephoneStatusType = penum.TelephoneStatusType_TelephoneStatusType_Finished
			if err := opuser.UpsertUserTelephone(ctx, suid, telephone); err != nil {
				return nil, err
			}
			updated.Telephones = append(updated.Telephones, telephone)
		}
		if err := reconcileNarrativeQueues(ctx, suid, time.Now().UnixMilli(), updated); err != nil {
			return nil, err
		}
		response = &papi.TelephoneFinishesResponse{
			IsReliabilityUp: false,
			CommonResponse:  viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func reconcileNarrativeQueues(
	ctx context.Context,
	suid string,
	now int64,
	updated *papi.UpdatedData,
) error {
	return viber.ReconcileNarrativeSchedules(ctx, suid, now, updated, viber.NarrativeUnlockNone, "")
}
