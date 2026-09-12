package mission

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

// No authoritative URLTransition capture is available; do not visit the
// submitted URL or invent mission progress for it.
func (s *MissionServer) URLTransition(ctx context.Context, req *papi.MissionURLTransitionRequest) (*papi.MissionURLTransitionResponse, error) {
	return &papi.MissionURLTransitionResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{})}, nil
}
