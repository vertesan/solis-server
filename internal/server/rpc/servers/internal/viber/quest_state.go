package viber

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// ReconcileQuestDailyCounters preserves historical clear timestamps/scores and
// resets only stale counters. The caller owns the transaction and loaded rows.
func ReconcileQuestDailyCounters(ctx context.Context, suid string, now int64, masters []*pmaster.Area, areas []*ptransaction.UserArea, quests []*ptransaction.UserQuest, updated *papi.UpdatedData) error {
	dailyAreas, dailyQuests := map[string]bool{}, map[string]bool{}
	for _, area := range masters {
		if area.GetType() != penum.AreaType_AreaType_Daily || area.GetDailySpecialTicketItemId() != "" {
			continue
		}
		dailyAreas[area.GetId()] = true
		for _, id := range area.GetQuestIds() {
			dailyQuests[id] = true
		}
	}
	for _, area := range areas {
		if dailyAreas[area.GetAreaId()] && area.GetDailyClearCount() != 0 && !SameJSTDay(area.GetLastClearedTime(), now) {
			area.DailyClearCount = 0
			if err := opuser.UpsertUserArea(ctx, suid, area); err != nil {
				return err
			}
			updated.Areas = append(updated.Areas, area)
		}
	}
	for _, quest := range quests {
		if dailyQuests[quest.GetQuestId()] && quest.GetDailyClearCount() != 0 && !SameJSTDay(quest.GetLastClearedTime(), now) {
			quest.DailyClearCount = 0
			if err := opuser.UpsertUserQuest(ctx, suid, quest); err != nil {
				return err
			}
			updated.Quests = append(updated.Quests, quest)
		}
	}
	return nil
}
