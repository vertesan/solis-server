package servers

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TelephoneServer struct {
	papi.UnimplementedTelephoneServer
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
