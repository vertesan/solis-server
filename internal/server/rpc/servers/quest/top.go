package quest

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *QuestServer) Top(ctx context.Context, _ *papi.Empty) (*papi.QuestTopResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.QuestTopResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		catalog, err := loadQuestCatalog(ctx, suid, questNow(), updated)
		if err != nil {
			return nil, err
		}
		hierarchy, state, changed, err := viber.ReconcileHierarchyNotification(ctx, suid, viber.HierarchyPrivateServerRank)
		if err != nil {
			return nil, err
		}
		if changed {
			updated.Hierarchy = state
		}
		response = &papi.QuestTopResponse{
			MainAreaGroups: catalog.Main, DailyAreas: catalog.Daily, TowerArea: catalog.Tower.GetArea(), ContestArea: catalog.Contest,
			CurrentTowerQuestId: catalog.Tower.GetCurrentTowerQuestId(), IsDailyPlayable: catalog.DailyPlayable,
			ContestTotalScore: catalog.ContestScore, HasUnplayedContest: catalog.HasUnplayed, IsTowerTopUnlocked: catalog.TowerUnlocked,
			PvpInfo: &papi.QuestTopPvpInfo{}, HierarchyNotiInfo: hierarchy, BulkDailyInfos: []*papi.QuestBulkDailyInfo{}, CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
