package servers

import (
	"context"
	"vertesan/solis-server/makino/home"
	"vertesan/solis-server/network/rpc/internal/metadata"
	pb "vertesan/solis-server/proto/papi"
	"vertesan/solis-server/utils/rich"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type HomeServer struct {
	pb.UnimplementedHomeServer
}

func (s *HomeServer) Login(ctx context.Context, req *pb.HomeLoginRequest) (*pb.HomeLoginResponse, error) {
	suid, err := metadata.GetSuid(ctx)
	if err != nil {
		rich.Error("failed to get suid: %v", err)
		return nil, status.Error(codes.Unauthenticated, "bad token")
	}
	resp, err := home.Login(context.Background(), suid.String())
	if err != nil {
		rich.Error("failed to login: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return resp, nil
}

// func (s *HomeServer) Enter(ctx context.Context, req *pb.HomeEnterRequest) (*pb.HomeEnterResponse, error) {

// }
