package notice

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *NoticeServer) Get(ctx context.Context, req *papi.NoticeGetRequest) (*papi.NoticeGetResponse, error) {
	notice := viber.NewDummyNoticeInfo()
	if req.GetNoticeId() != "" {
		catalog, err := findNoticeCatalog(ctx)
		if err != nil {
			return nil, err
		}
		for _, group := range [][]*papi.NoticeInfo{catalog.Notices, catalog.MalfunctionNotices, catalog.PrNotices, {catalog.ShowcaseShopNotice}} {
			for _, entry := range group {
				if entry.GetId() == req.GetNoticeId() {
					notice = entry
				}
			}
		}
	}
	return &papi.NoticeGetResponse{
		Notice:              notice,
		RoootAssociateToken: "",
		CommonResponse:      viber.NewCommonResponse(nil),
	}, nil
}
