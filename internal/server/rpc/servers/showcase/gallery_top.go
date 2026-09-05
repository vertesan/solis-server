package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ShowcaseServer) GalleryTop(ctx context.Context, _ *papi.Empty) (*papi.ShowcaseGalleryTopResponse, error) {
	return &papi.ShowcaseGalleryTopResponse{
		ShowcaseInfos:  []*papi.ShowcaseInfo{},
		TermInfo:       currentShowcaseTermInfo(),
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
