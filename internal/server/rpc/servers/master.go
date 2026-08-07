package servers

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/depot"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MasterServer struct {
	pb.UnimplementedMasterServer
}

func (s *MasterServer) Get(ctx context.Context, req *pb.Empty) (*pb.MasterGetResponse, error) {
	masterTagPacks, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to GetAllMasterTagPack: %v", err)
	}
	return &pb.MasterGetResponse{
		MasterTag: &ptransaction.MasterTag{
			Version:        depot.ServerProfile.MasterVersion,
			MasterTagPacks: masterTagPacks,
		},
	}, nil
}
