package servers

import (
	"context"
	pb "vertesan/solis-server/proto/papi"
)

type DokanServer struct {
	pb.UnimplementedDokanServer
}

func (s *DokanServer) List(ctx context.Context, _ *pb.Empty) (*pb.DokanListResponse, error) {
	return &pb.DokanListResponse{}, nil
}
