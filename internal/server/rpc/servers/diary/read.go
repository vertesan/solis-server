package diary

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DiaryServer) Read(ctx context.Context, req *papi.DiaryReadRequest) (*papi.DiaryReadResponse, error) {
	if len(req.GetDiaryIds()) == 0 {
		return nil, viber.InvalidArgumentf("at least one diaryId is required")
	}
	catalog, err := loadDiaryCatalog()
	if err != nil {
		return nil, err
	}
	validIds := make(map[string]struct{}, len(catalog))
	for _, info := range catalog {
		validIds[info.Id] = struct{}{}
	}
	requested := make(map[string]struct{}, len(req.DiaryIds))
	for _, id := range req.DiaryIds {
		if id == "" {
			return nil, viber.InvalidArgumentf("diaryId is required")
		}
		if _, exists := validIds[id]; !exists {
			return nil, viber.InvalidArgumentf("unknown diaryId %q", id)
		}
		requested[id] = struct{}{}
	}

	suid := viber.GetSuid(ctx).String()
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := findDiaryStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		if state.AllRead {
			return nil, nil
		}
		seen := make(map[string]struct{}, len(state.ReadIds)+len(requested))
		for _, id := range state.ReadIds {
			seen[id] = struct{}{}
		}
		for _, info := range catalog {
			if _, exists := requested[info.Id]; !exists {
				continue
			}
			if _, exists := seen[info.Id]; exists {
				continue
			}
			seen[info.Id] = struct{}{}
			state.ReadIds = append(state.ReadIds, info.Id)
		}
		return nil, opuser.UpsertUserDiaryState(ctx, state)
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.DiaryReadResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
