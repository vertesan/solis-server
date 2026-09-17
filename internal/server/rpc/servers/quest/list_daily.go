package quest

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *QuestServer) ListDaily(ctx context.Context, _ *papi.Empty) (*papi.QuestListDailyResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.QuestListDailyResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		catalog, err := loadQuestCatalog(ctx, suid, questNow(), updated)
		if err != nil {
			return nil, err
		}
		bulk, err := loadQuestBulkDailyEntries(ctx, suid, catalog)
		if err != nil {
			return nil, err
		}
		response = &papi.QuestListDailyResponse{Areas: catalog.Daily, BulkDailyInfos: questBulkDailyInfos(bulk), CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
