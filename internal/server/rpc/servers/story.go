package servers

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/internal/viber"
)

type StoryServer struct {
	pb.UnimplementedStoryServer
}

func (s *StoryServer) ListEvent(ctx context.Context, _ *pb.Empty) (*pb.StoryListEventResponse, error) {
	suid := viber.GetSuid(ctx).String()
	infos, err := opuser.FindAllUserEventStoryInfos(ctx, suid)
	if err != nil {
		return nil, err
	}
	return &pb.StoryListEventResponse{
		Infos:          infos,
		CommonResponse: newCommonResponse(nil),
	}, nil
}
