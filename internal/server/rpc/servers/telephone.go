package servers

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

type TelephoneServer struct {
	papi.UnimplementedTelephoneServer
}

func (s *TelephoneServer) Receive(
	ctx context.Context,
	req *papi.TelephoneReceiveRequest,
) (*papi.TelephoneReceiveResponse, error) {
	if len(req.GetTelephoneIds()) == 0 {
		return nil, viber.InvalidArgumentf("at least one telephoneId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.TelephoneReceiveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
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
			switch telephone.GetTelephoneStatusType() {
			case penum.TelephoneStatusType_TelephoneStatusType_Received,
				penum.TelephoneStatusType_TelephoneStatusType_Finished:
				continue
			case penum.TelephoneStatusType_TelephoneStatusType_Scheduled:
			default:
				return nil, status.Errorf(codes.FailedPrecondition, "telephone %q is not scheduled", telephoneID)
			}
			if telephone.GetScheduledTime() <= 0 || telephone.GetScheduledTime() > now {
				return nil, status.Errorf(codes.FailedPrecondition, "telephone %q has not arrived", telephoneID)
			}
			telephone.TelephoneStatusType = penum.TelephoneStatusType_TelephoneStatusType_Received
			if err := opuser.UpsertUserTelephone(ctx, suid, telephone); err != nil {
				return nil, err
			}
			updated.Telephones = append(updated.Telephones, telephone)
		}
		response = &papi.TelephoneReceiveResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

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
		if err := viber.ReconcileNarrativeQueues(ctx, suid, time.Now().UnixMilli(), updated); err != nil {
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

func (s *TelephoneServer) CallHistory(
	ctx context.Context,
	req *papi.TelephoneCallHistoryRequest,
) (*papi.TelephoneCallHistoryResponse, error) {
	if req.GetTelephoneId() == "" {
		return nil, viber.InvalidArgumentf("telephoneId is required")
	}
	if _, err := opmaster.FindTelephone(ctx, req.TelephoneId); err != nil {
		return nil, viber.GameplayError(err)
	}
	telephone, err := opuser.FindUserTelephone(ctx, viber.GetSuid(ctx).String(), req.TelephoneId)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if telephone.TelephoneStatusType != penum.TelephoneStatusType_TelephoneStatusType_Finished {
		return nil, status.Error(codes.PermissionDenied, "telephone history is unavailable before the call is finished")
	}
	return &papi.TelephoneCallHistoryResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
