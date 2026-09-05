package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ShowcaseServer) DeleteMyset(ctx context.Context, req *papi.ShowcaseDeleteMysetRequest) (*papi.ShowcaseDeleteMysetResponse, error) {
	if !validShowcaseMysetNumber(req.GetNumber()) {
		return nil, viber.InvalidArgumentf("valid number is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseDeleteMysetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := opuser.DeleteUserShowcaseMyset(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserShowcaseMysetLayout(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		common := viber.NewCommonResponse(nil)
		common.DeletedData.ShowcaseMysets = []*ptransaction.UserShowcaseMyset{{Number: req.GetNumber()}}
		response = &papi.ShowcaseDeleteMysetResponse{CommonResponse: common}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
