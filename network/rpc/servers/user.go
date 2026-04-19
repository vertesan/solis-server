package servers

import (
	"context"
	"vertesan/solis-server/makino/user"
	"vertesan/solis-server/network/rpc/internal"
	pb "vertesan/solis-server/proto/papi"
	"vertesan/solis-server/utils/rich"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserServer struct {
	pb.UnimplementedUserServer
}

func (s *UserServer) Get(ctx context.Context, _ *pb.Empty) (*pb.UserGetResponse, error) {
	suid, err := internal.GetSuid(ctx)
	if err != nil {
		rich.Error("failed to get suid: %v", err)
		return nil, status.Error(codes.Unauthenticated, "bad token")
	}
	return user.GetUser(context.Background(), suid.String())
}
