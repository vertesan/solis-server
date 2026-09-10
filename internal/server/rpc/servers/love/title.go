package love

import (
	"context"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"time"
)

func (s *LoveServer) Title(ctx context.Context, _ *papi.Empty) (*papi.LoveTitleResponse, error) {
	response := &papi.LoveTitleResponse{LoveInfos: []*papi.LoveInfo{}, CommonResponse: viber.NewCommonResponse(nil)}
	for _, id := range moshikoi.SupportedLoveIDs() {
		catalog, err := moshikoi.LoadCatalog(ctx, id)
		if err != nil {
			return nil, err
		}
		info, err := viber.BuildLoveInfo(ctx, viber.GetSuid(ctx).String(), catalog, time.Now().UnixMilli())
		if err != nil {
			return nil, err
		}
		response.LoveInfos = append(response.LoveInfos, info)
	}
	return response, nil
}
