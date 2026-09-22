package viber

import (
	"context"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

type DreamMissionProgress struct {
	Generated, Cleared int64
	Difficulty         int32
	RankType           penum.DreamAreaRankType
}

func CheckDreamUnlocked(ctx context.Context, suid string) error {
	unlocked, err := NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli()).Satisfied(dream.UnlockConditionID)
	if err != nil {
		return err
	}
	if !unlocked {
		return FailedPreconditionf("image training is not unlocked")
	}
	return nil
}

// ReconcileDream shares the monthly boundary with Mission and Home. Area
// progress and normal/company missions never reset at this boundary.
func ReconcileDream(ctx context.Context, suid string, now int64, updated *papi.UpdatedData) (*dream.MonthlyState, error) {
	monthly, err := dream.ReconcileMonthly(ctx, suid, now, updated)
	if err != nil {
		return nil, err
	}
	catalog, err := LoadActiveMissionCatalog(ctx, suid, now)
	if err != nil {
		return nil, err
	}
	if err := catalog.Reconcile(ctx, suid, now, updated); err != nil {
		return nil, err
	}
	return monthly, nil
}

func AdvanceDreamMissions(ctx context.Context, suid string, progress DreamMissionProgress, now int64, updated *papi.UpdatedData) error {
	if progress.Generated < 0 || progress.Cleared < 0 || progress.Difficulty < 0 {
		return InvalidArgumentf("Dream mission progress cannot be negative")
	}
	monthly, err := ReconcileDream(ctx, suid, now, updated)
	if err != nil {
		return err
	}
	state, err := opuser.FindUserDreamState(ctx, suid)
	if err != nil {
		return err
	}
	total, err := opuser.FindUserTotalCount(ctx, suid)
	if err != nil {
		return err
	}
	if total.GetDreamAreaGenerateCount() < 0 || total.GetDreamAreaGenerateCount() > math.MaxInt64-progress.Generated ||
		state.QuestClearCount < 0 || state.QuestClearCount > math.MaxInt64-progress.Cleared {
		return FailedPreconditionf("Dream lifetime counter would overflow")
	}
	total.DreamAreaGenerateCount += progress.Generated
	state.QuestClearCount += progress.Cleared
	state.MaxDifficulty = max(state.MaxDifficulty, progress.Difficulty)
	if progress.Generated > 0 {
		if err := opuser.UpsertUserTotalCount(ctx, suid, total); err != nil {
			return err
		}
		updated.TotalCount = total
	}
	if err := opuser.UpsertUserDreamState(ctx, state); err != nil {
		return err
	}
	updates := []struct {
		kind   penum.MissionType
		value  int64
		reach  bool
		target string
	}{
		{penum.MissionType_MissionType_DreamAreaGenerateCount, progress.Generated, false, monthly.SeasonID},
		{penum.MissionType_MissionType_DreamAreaGenerateTotalCount, total.GetDreamAreaGenerateCount(), true, ""},
		{penum.MissionType_MissionType_DreamQuestClearCount, progress.Cleared, false, monthly.SeasonID},
		{penum.MissionType_MissionType_DreamQuestClearTotalCount, state.QuestClearCount, true, ""},
		{penum.MissionType_MissionType_DreamQuestClearDifficultyLevelAchievement, int64(state.MaxDifficulty), true, ""},
		{penum.MissionType_MissionType_DreamSeasonQuestClearDifficultyLevelAchievement, int64(progress.Difficulty), true, monthly.SeasonID},
	}
	if progress.Generated > 0 && progress.RankType >= penum.DreamAreaRankType_DreamAreaRankType_A {
		// A targetless ConditionClear mission is not a wildcard for this
		// event-specific achievement. Only advance this month's grade marker.
		conditionID := "cd-" + monthly.SeasonID + "-season-max-grade-a"
		for _, master := range monthly.Missions {
			if master.GetMissionType() != penum.MissionType_MissionType_ConditionClear || !containsString(master.GetTargetIds(), conditionID) {
				continue
			}
			row, err := opuser.FindUserMission(ctx, suid, master.GetId())
			if err != nil {
				return err
			}
			completed, changed, err := reachMissionProgress(row, master, 1, now)
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			if completed {
				row.StatusType = penum.MissionStatusType_MissionStatusType_NotReceived
			}
			if err := opuser.UpsertUserMission(ctx, suid, row); err != nil {
				return err
			}
			PutUpdatedMission(updated, row)
		}
	}
	for _, update := range updates {
		if update.value <= 0 {
			continue
		}
		var rows []*ptransaction.UserMission
		if update.reach {
			rows, err = ReachUserMissions(ctx, suid, update.kind, update.value, update.target)
		} else {
			rows, err = AdvanceUserMissions(ctx, suid, update.kind, update.value, update.target)
		}
		if err != nil {
			return err
		}
		for _, row := range rows {
			PutUpdatedMission(updated, row)
		}
	}
	return nil
}
