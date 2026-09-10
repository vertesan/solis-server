package love

import (
	"context"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"time"
)

func (s *LoveServer) TitleExtra(ctx context.Context, req *papi.LoveTitleExtraRequest) (*papi.LoveTitleExtraResponse, error) {
	catalog, err := moshikoi.LoadCatalog(ctx, req.GetLoveId())
	if err != nil {
		return nil, err
	}
	info, err := viber.BuildLoveInfo(ctx, viber.GetSuid(ctx).String(), catalog, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	return &papi.LoveTitleExtraResponse{LoveInfo: info, TitleAssetId: catalog.TitleAssetID, CommonResponse: viber.NewCommonResponse(nil)}, nil
}
