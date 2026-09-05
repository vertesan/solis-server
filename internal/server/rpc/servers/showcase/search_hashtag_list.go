package showcase

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ShowcaseServer) SearchHashtagList(ctx context.Context, req *papi.ShowcaseSearchHashtagListRequest) (*papi.ShowcaseSearchHashtagListResponse, error) {
	if req.GetHashtagId() == "" {
		return nil, viber.InvalidArgumentf("hashtagId is required")
	}
	hashtags, err := opmaster.FindAllShowcaseHashtags(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if !slices.ContainsFunc(hashtags, func(value *pmaster.ShowcaseHashtag) bool {
		return value.GetId() == req.GetHashtagId()
	}) {
		return nil, status.Error(codes.NotFound, "showcase hashtag was not found")
	}
	return &papi.ShowcaseSearchHashtagListResponse{
		ShowcaseInfos:          []*papi.ShowcaseInfo{},
		TermInfo:               currentShowcaseTermInfo(),
		IsEndHashtagSearchList: true,
		IsAlwaysEmpty:          true,
		CommonResponse:         viber.NewCommonResponse(nil),
	}, nil
}
