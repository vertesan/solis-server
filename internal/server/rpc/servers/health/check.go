package health

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
)

func (s *HealthServer) Check(context.Context, *papi.HealthCheckRequest) (*papi.HealthCheckResponse, error) {
	return &papi.HealthCheckResponse{}, nil
}
