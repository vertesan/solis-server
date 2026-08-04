package servers

import (
	"context"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
)

type DokanServer struct {
	pb.UnimplementedDokanServer
}

func (s *DokanServer) List(ctx context.Context, _ *pb.Empty) (*pb.DokanListResponse, error) {
	return &pb.DokanListResponse{}, nil
}
