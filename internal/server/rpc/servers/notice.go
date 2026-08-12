package servers

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const noticePageSize = 20

type NoticeServer struct {
	pb.UnimplementedNoticeServer
}

func (s *NoticeServer) List(ctx context.Context, _ *pb.Empty) (*pb.NoticeListResponse, error) {
	catalog, err := findNoticeCatalog(ctx)
	if err != nil {
		return nil, err
	}

	notices, noticeHasNext := firstNoticePage(catalog.Notices)
	malfunctionNotices, malfunctionNoticeHasNext := firstNoticePage(catalog.MalfunctionNotices)
	prNotices, prNoticeHasNext := firstNoticePage(catalog.PrNotices)
	return &pb.NoticeListResponse{
		Notices:                  notices,
		MalfunctionNotices:       malfunctionNotices,
		PrNotices:                prNotices,
		NoticeHasNext:            noticeHasNext,
		MalfunctionNoticeHasNext: malfunctionNoticeHasNext,
		PrNoticeHasNext:          prNoticeHasNext,
		CommonResponse:           newCommonResponse(nil),
	}, nil
}

func (s *NoticeServer) FetchList(ctx context.Context, req *pb.NoticeFetchRequest) (*pb.NoticeFetchResponse, error) {
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

	var notices []*pb.NoticeInfo
	switch penum.NoticeCategoryType(req.GetNoticeCategoryType()) {
	case penum.NoticeCategoryType_NoticeCategoryType_Notice:
		notices = catalog.Notices
	case penum.NoticeCategoryType_NoticeCategoryType_Malfunction:
		notices = catalog.MalfunctionNotices
	case penum.NoticeCategoryType_NoticeCategoryType_Pr:
		notices = catalog.PrNotices
	}

	page, hasNext := noticePage(notices, int(req.GetOffset()))
	return &pb.NoticeFetchResponse{
		Notices:        page,
		HasNext:        hasNext,
		CommonResponse: newCommonResponse(nil),
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

func firstNoticePage(notices []*pb.NoticeInfo) ([]*pb.NoticeInfo, bool) {
	return noticePage(notices, 0)
}

func noticePage(notices []*pb.NoticeInfo, offset int) ([]*pb.NoticeInfo, bool) {
	if offset >= len(notices) {
		return []*pb.NoticeInfo{}, false
	}
	end := offset + noticePageSize
	if end < len(notices) {
		return notices[offset:end], true
	}
	return notices[offset:], false
}
