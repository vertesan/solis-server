package servers

import (
	"context"

	"github.com/vertesan/solis-server/internal/makino/user"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/internal/viber"
)

type UserServer struct {
	pb.UnimplementedUserServer
}

func (s *UserServer) Get(ctx context.Context, _ *pb.Empty) (*pb.UserGetResponse, error) {
	suid := viber.GetSuid(ctx)
	return user.GetUser(context.Background(), suid.String())
}
