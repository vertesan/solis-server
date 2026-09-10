package love

import (
	"context"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *LoveServer) GetMainCharacter(ctx context.Context, req *papi.LoveGetMainCharacterRequest) (*papi.LoveGetMainCharacterResponse, error) {
	catalog, err := moshikoi.LoadCatalog(ctx, req.GetLoveId())
	if err != nil {
		return nil, err
	}
	return &papi.LoveGetMainCharacterResponse{MainCharacterInfo: catalog.MainCharacterInfo(), CommonResponse: viber.NewCommonResponse(nil)}, nil
}
