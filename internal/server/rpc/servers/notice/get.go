package notice

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *NoticeServer) Get(ctx context.Context, req *papi.NoticeGetRequest) (*papi.NoticeGetResponse, error) {
	return &papi.NoticeGetResponse{
		Notice:              viber.NewDummyNoticeInfo(),
		RoootAssociateToken: "",
		CommonResponse:      viber.NewCommonResponse(nil),
	}, nil
}
