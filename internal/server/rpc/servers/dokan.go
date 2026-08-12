package servers

import (
	"context"

	pb "github.com/vertesan/solis-server/internal/proto/papi"
)

type DokanServer struct {
	pb.UnimplementedDokanServer
}

func (s *DokanServer) List(_ context.Context, _ *pb.Empty) (*pb.DokanListResponse, error) {
	return &pb.DokanListResponse{
		DokanInfos:     []*pb.DokanInfo{},
		IsNeedReCall:   false,
		CommonResponse: newCommonResponse(nil),
	}, nil
}

func (s *DokanServer) SetWatched(_ context.Context, _ *pb.DokanSetWatchedRequest) (*pb.DokanSetWathcedResponse, error) {
	return &pb.DokanSetWathcedResponse{CommonResponse: newCommonResponse(nil)}, nil
}

func (s *DokanServer) OutputWatchedLog(_ context.Context, _ *pb.DokanOutputWatchedLogRequest) (*pb.DokanOutputWatchedLogResponse, error) {
	return &pb.DokanOutputWatchedLogResponse{CommonResponse: newCommonResponse(nil)}, nil
}
