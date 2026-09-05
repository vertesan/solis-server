package system

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
)

func (s *SystemServer) CheckApp(ctx context.Context, _ *papi.Empty) (*papi.Empty, error) {
	return &papi.Empty{}, nil
}
