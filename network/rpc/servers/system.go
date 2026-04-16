package servers

import (
	"context"
	pb "vertesan/solis-server/proto/papi"
)

type SystemServer struct {
	pb.UnimplementedSystemServer
}

func (s *SystemServer) Check(ctx context.Context, req *pb.SystemCheckRequest) (*pb.SystemCheckResponse, error) {
	return &pb.SystemCheckResponse{
		MaintenanceInfo: &pb.MaintenanceInfo{
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
		ReviewInfo: &pb.ReviewInfo{
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
