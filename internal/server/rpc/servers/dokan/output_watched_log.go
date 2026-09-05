package dokan

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DokanServer) OutputWatchedLog(_ context.Context, _ *papi.DokanOutputWatchedLogRequest) (*papi.DokanOutputWatchedLogResponse, error) {
	return &papi.DokanOutputWatchedLogResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
