package activity

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ActivityServer) LoadRefresh(ctx context.Context, _ *papi.Empty) (*papi.ActivityLoadRefreshResponse, error) {
	progress, err := opuser.FindUserRefreshRun(ctx, viber.GetSuid(ctx).String())
	if err != nil {
		return nil, viber.GameplayError(activityRunError("refresh", err))
	}
	return &papi.ActivityLoadRefreshResponse{
		Progress:       progress,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
