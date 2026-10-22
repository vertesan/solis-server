package master

import (
	"context"

	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *MasterServer) Get(ctx context.Context, req *papi.Empty) (*papi.MasterGetResponse, error) {
	snapshot := masterdata.Current()
	if snapshot == nil {
		return nil, status.Error(codes.Unavailable, "master snapshot is not ready")
	}
	return &papi.MasterGetResponse{
		MasterTag: snapshot.Tag(),
	}, nil
}
