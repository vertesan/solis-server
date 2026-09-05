package dokan

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DokanServer) SetWatched(_ context.Context, _ *papi.DokanSetWatchedRequest) (*papi.DokanSetWathcedResponse, error) {
	return &papi.DokanSetWathcedResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
