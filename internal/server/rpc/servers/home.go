package servers

import (
	"context"

	"github.com/vertesan/solis-server/internal/makino/home"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/internal/viber"
)

type HomeServer struct {
	pb.UnimplementedHomeServer
}

func (s *HomeServer) Login(ctx context.Context, req *pb.HomeLoginRequest) (*pb.HomeLoginResponse, error) {
	suid := viber.GetSuid(ctx)
	return home.Login(context.Background(), suid)
}

func (s *HomeServer) Enter(ctx context.Context, req *pb.HomeEnterRequest) (*pb.HomeEnterResponse, error) {
	suid := viber.GetSuid(ctx)
	return home.Enter(context.Background(), suid)
}
