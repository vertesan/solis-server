package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ShowcaseServer) SearchFriendRecommendList(ctx context.Context, _ *papi.Empty) (*papi.ShowcaseSearchFriendRecommendListResponse, error) {
	return &papi.ShowcaseSearchFriendRecommendListResponse{
		ShowcaseInfos:  []*papi.ShowcaseInfo{},
		TermInfo:       currentShowcaseTermInfo(),
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
