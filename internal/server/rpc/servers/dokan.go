package servers

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

type DokanServer struct {
	papi.UnimplementedDokanServer
}

func (s *DokanServer) List(_ context.Context, _ *papi.Empty) (*papi.DokanListResponse, error) {
	return &papi.DokanListResponse{
		DokanInfos:     []*papi.DokanInfo{},
		IsNeedReCall:   false,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *DokanServer) SetWatched(_ context.Context, _ *papi.DokanSetWatchedRequest) (*papi.DokanSetWathcedResponse, error) {
	return &papi.DokanSetWathcedResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func (s *DokanServer) OutputWatchedLog(_ context.Context, _ *papi.DokanOutputWatchedLogRequest) (*papi.DokanOutputWatchedLogResponse, error) {
	return &papi.DokanOutputWatchedLogResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
