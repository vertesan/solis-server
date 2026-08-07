package servers

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/utils/mcontext"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MasterServer struct {
	pb.UnimplementedMasterServer
}

func (s *MasterServer) Get(ctx context.Context, req *pb.Empty) (*pb.MasterGetResponse, error) {
	logger := mcontext.GetLogger(ctx)
	masterTagPacks, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		logger.Error("failed to GetAllMasterTagPack", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &pb.MasterGetResponse{
		MasterTag: &ptransaction.MasterTag{
			Version:        depot.ServerProfile.MasterVersion,
			MasterTagPacks: masterTagPacks,
		},
	}, nil
}
