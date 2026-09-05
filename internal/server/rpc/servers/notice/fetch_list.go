package notice

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *NoticeServer) FetchList(ctx context.Context, req *papi.NoticeFetchRequest) (*papi.NoticeFetchResponse, error) {
	if req.GetOffset() < 0 {
		return nil, status.Error(codes.InvalidArgument, "offset must not be negative")
	}

	switch penum.NoticeCategoryType(req.GetNoticeCategoryType()) {
	case penum.NoticeCategoryType_NoticeCategoryType_Notice:
	case penum.NoticeCategoryType_NoticeCategoryType_Malfunction:
	case penum.NoticeCategoryType_NoticeCategoryType_Pr:
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown notice category type")
	}

	catalog, err := findNoticeCatalog(ctx)
	if err != nil {
		return nil, err
	}

	var notices []*papi.NoticeInfo
	switch penum.NoticeCategoryType(req.GetNoticeCategoryType()) {
	case penum.NoticeCategoryType_NoticeCategoryType_Notice:
		notices = catalog.Notices
	case penum.NoticeCategoryType_NoticeCategoryType_Malfunction:
		notices = catalog.MalfunctionNotices
	case penum.NoticeCategoryType_NoticeCategoryType_Pr:
		notices = catalog.PrNotices
	}

	page, hasNext := noticePage(notices, int(req.GetOffset()))
	return &papi.NoticeFetchResponse{
		Notices:        page,
		HasNext:        hasNext,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
