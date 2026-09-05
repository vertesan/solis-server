package system

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
)

func (s *SystemServer) Check(ctx context.Context, req *papi.SystemCheckRequest) (*papi.SystemCheckResponse, error) {
	return &papi.SystemCheckResponse{
		MaintenanceInfo: &papi.MaintenanceInfo{
			InMaintenance:   false,
			TopMessage:      "",
			BottomMessage:   "",
			StartTime:       0,
			EndTime:         0,
			IsBeforeRelease: false,
			TwitterUrl:      "",
			MvUrl:           "",
			GamePvUrl:       "",
		},
		ReviewInfo: &papi.ReviewInfo{
			InReview:           false,
			AlternativeApiHost: "",
		},
		KeepAliveTimeMilliseconds:             60000,
		KeepAliveTimeoutMilliseconds:          5000,
		EnableCrashlyticsCollection:           false,
		EnableUnityLogErrorToLogException:     false,
		EnableApiParallelExecutionSuppression: false,
	}, nil
}
