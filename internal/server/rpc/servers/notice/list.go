package notice

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *NoticeServer) List(ctx context.Context, _ *papi.Empty) (*papi.NoticeListResponse, error) {
	catalog, err := findNoticeCatalog(ctx)
	if err != nil {
		return nil, err
	}

	notices, noticeHasNext := firstNoticePage(catalog.Notices)
	malfunctionNotices, malfunctionNoticeHasNext := firstNoticePage(catalog.MalfunctionNotices)
	prNotices, prNoticeHasNext := firstNoticePage(catalog.PrNotices)
	return &papi.NoticeListResponse{
		Notices:                  notices,
		MalfunctionNotices:       malfunctionNotices,
		PrNotices:                prNotices,
		NoticeHasNext:            noticeHasNext,
		MalfunctionNoticeHasNext: malfunctionNoticeHasNext,
		PrNoticeHasNext:          prNoticeHasNext,
		CommonResponse:           viber.NewCommonResponse(nil),
	}, nil
}

func firstNoticePage(notices []*papi.NoticeInfo) ([]*papi.NoticeInfo, bool) {
	return noticePage(notices, 0)
}
