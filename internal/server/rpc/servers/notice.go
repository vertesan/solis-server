package servers

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const noticePageSize = 20

type NoticeServer struct {
	papi.UnimplementedNoticeServer
}

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

func findNoticeCatalog(ctx context.Context) (*smodel.NoticeCatalog, error) {
	catalog, err := opsolis.FindNoticeCatalog(ctx)
	if err == nil {
		return catalog, nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &smodel.NoticeCatalog{}, nil
	}
	return nil, err
}

func firstNoticePage(notices []*papi.NoticeInfo) ([]*papi.NoticeInfo, bool) {
	return noticePage(notices, 0)
}

func noticePage(notices []*papi.NoticeInfo, offset int) ([]*papi.NoticeInfo, bool) {
	if offset >= len(notices) {
		return []*papi.NoticeInfo{}, false
	}
	end := offset + noticePageSize
	if end < len(notices) {
		return notices[offset:end], true
	}
	return notices[offset:], false
}
