package diary

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DiaryServer) GetInfos(ctx context.Context, _ *papi.Empty) (*papi.DiaryGetInfosResponse, error) {
	infos, err := loadDiaryCatalog()
	if err != nil {
		return nil, err
	}
	state, err := findDiaryStateOrEmpty(ctx, viber.GetSuid(ctx).String())
	if err != nil {
		return nil, err
	}
	readIds := make(map[string]struct{}, len(state.ReadIds))
	for _, id := range state.ReadIds {
		readIds[id] = struct{}{}
	}
	for _, info := range infos {
		_, info.IsRead = readIds[info.Id]
		info.IsRead = state.AllRead || info.IsRead
	}
	return &papi.DiaryGetInfosResponse{Infos: infos, CommonResponse: viber.NewCommonResponse(nil)}, nil
}
