package dokan

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DokanServer) List(_ context.Context, _ *papi.Empty) (*papi.DokanListResponse, error) {
	return &papi.DokanListResponse{
		DokanInfos:     []*papi.DokanInfo{},
		IsNeedReCall:   false,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
