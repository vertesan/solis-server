package servers

import (
	"context"
	"github.com/vertesan/solis-server/internal/makino/user"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/internal/metadata"
	"github.com/vertesan/solis-server/internal/utils/rich"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserServer struct {
	pb.UnimplementedUserServer
}

func (s *UserServer) Get(ctx context.Context, _ *pb.Empty) (*pb.UserGetResponse, error) {
	suid, err := metadata.GetSuid(ctx)
	if err != nil {
		rich.Error("failed to get suid: %v", err)
		return nil, status.Error(codes.Unauthenticated, "bad token")
	}
	usr, err := user.GetUser(context.Background(), suid.String())
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return usr, nil
}
