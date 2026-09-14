package quest

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *QuestServer) ListTower(ctx context.Context, _ *papi.Empty) (*papi.QuestListTowerResponse, error) {
	var response *papi.QuestListTowerResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		catalog, err := loadQuestCatalog(ctx, viber.GetSuid(ctx).String(), questNow(), updated)
		if err != nil {
			return nil, err
		}
		response = &papi.QuestListTowerResponse{Area: catalog.Tower.GetArea(), CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
