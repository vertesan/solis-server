package quest

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *QuestServer) TowerTop(ctx context.Context, _ *papi.Empty) (*papi.QuestTowerTopResponse, error) {
	var response *papi.QuestTowerTopResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, err := loadQuestCatalog(ctx, viber.GetSuid(ctx).String(), questNow(), &papi.UpdatedData{})
		if err != nil {
			return nil, err
		}
		hierarchy, _, _, err := viber.ReconcileHierarchyNotification(ctx, viber.GetSuid(ctx).String(), viber.HierarchyPrivateServerRank)
		if err != nil {
			return nil, err
		}
		response = &papi.QuestTowerTopResponse{TowerArea: catalog.Tower, SubTowerAreas: catalog.SubTowers, HierarchyNotiInfo: hierarchy}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
