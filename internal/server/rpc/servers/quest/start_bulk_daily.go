package quest

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
)

func (s *QuestServer) StartBulkDaily(ctx context.Context, _ *papi.Empty) (*papi.QuestStartBulkDailyResponse, error) {
	ctx, err := mcontext.WithLiveRequest(ctx)
	if err != nil {
		return nil, err
	}
	_, started, _ := mcontext.GetLiveRequest(ctx)
	suid := viber.GetSuid(ctx).String()
	var response *papi.QuestStartBulkDailyResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		// Serialize selection and all component settlements with ordinary Start
		// and concurrent bulk calls. Nested helpers reuse this same transaction.
		if _, err := opuser.LockUserQuestStart(ctx, suid); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		catalog, err := loadQuestCatalog(ctx, suid, started.UnixMilli(), updated)
		if err != nil {
			return nil, err
		}
		entries, err := loadQuestBulkDailyEntries(ctx, suid, catalog)
		if err != nil {
			return nil, err
		}
		results := make([]*papi.QuestBulkDailyResultInfo, 0, len(entries))
		for _, entry := range entries {
			questID := entry.Info.GetQuestId()
			questCtx, err := mcontext.WithLiveRequestVariant(ctx, questID)
			if err != nil {
				return nil, err
			}
			result, err := startQuest(questCtx, &papi.QuestStartRequest{
				QuestId: questID, DeckNumber: entry.DeckNumber, PlayCount: entry.PlayCount, IsSkip: true,
				// UseDailyTicket deliberately remains false: bulk never spends items.
			}, &viber.LiveFormation{Name: entry.DeckName, Positions: entry.DeckPositions})
			if err != nil {
				// Validation/persistence errors roll back the entire batch. A valid
				// simulated defeat is not an error: it leaves this area's passes
				// unspent and does not prevent the other areas from completing.
				return nil, err
			}
			results = append(results, &papi.QuestBulkDailyResultInfo{
				QuestId: questID, PlayCount: entry.PlayCount, DeckPositions: entry.Info.GetDeckPositions(),
				Reward: result.GetReward(), RankType: result.GetRankType(), RankPlus: result.GetRankPlus(),
			})
			mergeQuestUpdates(updated, result.GetCommonResponse().GetUpdatedData())
		}
		response = &papi.QuestStartBulkDailyResponse{Results: results, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
