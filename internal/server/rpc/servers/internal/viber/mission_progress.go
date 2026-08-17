package viber

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// AdvanceUserMissions applies an action only to missions already active for
// the user. Mission schedules create those rows elsewhere; gameplay handlers
// must not reactivate expired or historical master missions by themselves.
func AdvanceUserMissions(
	ctx context.Context,
	suid string,
	missionType penum.MissionType,
	amount int64,
	targetIds ...string,
) ([]*ptransaction.UserMission, error) {
	return AdvanceUserMissionsWithAmount(ctx, suid, missionType, func(*pmaster.Mission) int64 { return amount }, targetIds...)
}

func AdvanceUserMissionsWithAmount(
	ctx context.Context,
	suid string,
	missionType penum.MissionType,
	amount func(*pmaster.Mission) int64,
	targetIds ...string,
) ([]*ptransaction.UserMission, error) {
	userMissions, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return nil, err
	}
	if len(userMissions) == 0 {
		return []*ptransaction.UserMission{}, nil
	}

	activeMissions := make([]*ptransaction.UserMission, 0, len(userMissions))
	ids := make([]string, 0, len(userMissions))
	userById := make(map[string]*ptransaction.UserMission, len(userMissions))
	for _, mission := range userMissions {
		if !missionCanProgress(mission) {
			continue
		}
		activeMissions = append(activeMissions, mission)
		ids = append(ids, mission.MissionId)
		userById[mission.MissionId] = mission
	}
	if len(activeMissions) == 0 {
		return []*ptransaction.UserMission{}, nil
	}
	masters, err := opmaster.FindMissionsByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	masterById := make(map[string]*pmaster.Mission, len(masters))
	for _, mission := range masters {
		masterById[mission.Id] = mission
	}

	now := time.Now().UnixMilli()
	changed := make(map[string]*ptransaction.UserMission)
	changedOrder := make([]string, 0)
	completed := make([]string, 0)
	for _, mission := range activeMissions {
		master := masterById[mission.MissionId]
		if master == nil || master.MissionType != missionType || !missionTargetsMatch(master.TargetIds, targetIds) {
			continue
		}
		increment := amount(master)
		if increment <= 0 {
			return nil, fmt.Errorf("mission progress amount must be positive")
		}
		newlyCompleted, didChange, err := addMissionProgress(mission, master, increment, now)
		if err != nil {
			return nil, err
		}
		if !didChange {
			continue
		}
		changed[mission.MissionId] = mission
		changedOrder = append(changedOrder, mission.MissionId)
		if newlyCompleted {
			completed = append(completed, mission.MissionId)
		}
	}

	// A newly-cleared mission contributes once to each active mission-clear
	// mission that names it. This may itself clear another mission, so process
	// the small dependency chain until no new completion is produced.
	for len(completed) > 0 {
		increments := make(map[string]int64)
		hasParent := make(map[string]bool, len(completed))
		for _, parent := range activeMissions {
			parentMaster := masterById[parent.MissionId]
			if parentMaster == nil || parentMaster.MissionType != penum.MissionType_MissionType_MissionClearCount || !missionCanProgress(parent) {
				continue
			}
			for _, childId := range completed {
				if containsString(parentMaster.TargetIds, childId) {
					increments[parent.MissionId]++
					hasParent[childId] = true
				}
			}
		}
		for _, childId := range completed {
			child := userById[childId]
			if hasParent[childId] {
				child.StatusType = penum.MissionStatusType_MissionStatusType_Clear
			} else {
				child.StatusType = penum.MissionStatusType_MissionStatusType_NotReceived
			}
		}

		nextCompleted := make([]string, 0)
		for parentId, increment := range increments {
			parent := userById[parentId]
			newlyCompleted, didChange, err := addMissionProgress(parent, masterById[parentId], increment, now)
			if err != nil {
				return nil, err
			}
			if !didChange {
				continue
			}
			if _, exists := changed[parentId]; !exists {
				changedOrder = append(changedOrder, parentId)
			}
			changed[parentId] = parent
			if newlyCompleted {
				nextCompleted = append(nextCompleted, parentId)
			}
		}
		completed = nextCompleted
	}

	result := make([]*ptransaction.UserMission, 0, len(changedOrder))
	for _, id := range changedOrder {
		mission := changed[id]
		if err := opuser.UpsertUserMission(ctx, suid, mission); err != nil {
			return nil, err
		}
		result = append(result, mission)
	}
	return result, nil
}

func addMissionProgress(
	mission *ptransaction.UserMission,
	master *pmaster.Mission,
	amount int64,
	now int64,
) (newlyCompleted bool, changed bool, err error) {
	if !missionCanProgress(mission) {
		return false, false, nil
	}
	if mission.Progress < 0 || mission.CurrentThreshold <= 0 {
		return false, false, fmt.Errorf("mission %q has invalid user progress", mission.MissionId)
	}
	maxThreshold := int64(0)
	currentThresholdExists := false
	for _, progress := range master.Progresses {
		if progress.Threshold > maxThreshold {
			maxThreshold = progress.Threshold
		}
		if progress.Threshold == mission.CurrentThreshold {
			currentThresholdExists = true
		}
	}
	if !currentThresholdExists || maxThreshold <= 0 {
		return false, false, fmt.Errorf("mission %q user threshold is absent from master data", mission.MissionId)
	}

	oldStatus := mission.StatusType
	if mission.Progress >= maxThreshold || amount > math.MaxInt64-mission.Progress {
		mission.Progress = maxThreshold
	} else {
		mission.Progress += amount
		if mission.Progress > maxThreshold {
			mission.Progress = maxThreshold
		}
	}
	mission.LastUpdateTime = now

	if oldStatus == penum.MissionStatusType_MissionStatusType_Clear && mission.Progress >= maxThreshold {
		mission.StatusType = penum.MissionStatusType_MissionStatusType_NotReceived
	}
	return oldStatus == penum.MissionStatusType_MissionStatusType_Progress && mission.Progress >= mission.CurrentThreshold, true, nil
}

func missionCanProgress(mission *ptransaction.UserMission) bool {
	return mission.StatusType == penum.MissionStatusType_MissionStatusType_Progress ||
		mission.StatusType == penum.MissionStatusType_MissionStatusType_Clear
}

func missionTargetsMatch(masterTargetIds, actionTargetIds []string) bool {
	if len(masterTargetIds) == 0 {
		return true
	}
	for _, targetId := range actionTargetIds {
		if containsString(masterTargetIds, targetId) {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
