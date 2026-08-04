package servers

import (
	"context"
	"github.com/vertesan/solis-server/internal/makino/home"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/internal/metadata"
	"github.com/vertesan/solis-server/internal/utils/rich"

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

func (s *HomeServer) Enter(ctx context.Context, req *pb.HomeEnterRequest) (*pb.HomeEnterResponse, error) {
	suid, err := metadata.GetSuid(ctx)
	if err != nil {
		rich.Error("failed to get suid: %v", err)
		return nil, status.Error(codes.Unauthenticated, "bad token")
	}
	resp, err := home.Enter(context.Background(), suid.String())
	if err != nil {
		rich.Error("failed to enter home: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return resp, nil
}
