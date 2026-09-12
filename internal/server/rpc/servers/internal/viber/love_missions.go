package viber

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

// PermanentLoveMissionCondition removes only calendar gates. A branch that
// contained only a date becomes unconditional, rather than an unsupported
// empty ConditionSetting (conditions are ORs of their settings).
func PermanentLoveMissionCondition(condition *pmaster.Condition) *pmaster.Condition {
	permanent := proto.Clone(condition).(*pmaster.Condition)
	for _, setting := range permanent.GetSettings() {
		if setting.GetTerm() == nil {
			continue
		}
		setting.Term = nil
		if proto.Size(setting) == 0 {
			permanent.Settings = nil
			break
		}
	}
	return permanent
}

// ReconcileLoveMissions activates the supported event's master-defined missions.
// Daily missions reset on JST midnight; loading a save never rewinds missions.
// Claims remain the responsibility of the Mission service, not Love rewards.
func ReconcileLoveMissions(ctx context.Context, suid, loveID string, exp int64, updated *papi.UpdatedData) error {
	catalog, err := moshikoi.LoadCatalog(ctx, loveID)
	if err != nil {
		return err
	}
	routes, err := moshikoi.LoadRoutes(loveID)
	if err != nil {
		return err
	}
	groups, err := LoveMissionGroups(ctx, catalog)
	if err != nil {
		return err
	}
	var ids []string
	for _, group := range groups {
		ids = append(ids, group.GetMissionIds()...)
	}
	masters, err := opmaster.FindMissionsByIds(ctx, ids)
	if err != nil {
		return err
	}
	users, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return err
	}
	byID := make(map[string]*ptransaction.UserMission, len(users))
	for _, user := range users {
		byID[user.GetMissionId()] = user
	}
	loves, err := opuser.FindAllUserLoves(ctx, suid)
	if err != nil {
		return err
	}
	var love *ptransaction.UserLove
	for _, candidate := range loves {
		if candidate.GetLoveId() == loveID {
			love = candidate
			break
		}
	}
	if love == nil {
		return nil // Merely seeing the event entrance does not register a player.
	}
	now := time.Now().UnixMilli()
	evaluator := NewCommerceConditionEvaluator(ctx, suid, now)
	for _, master := range masters {
		mission := byID[master.GetId()]
		changed := false
		if mission == nil || master.GetCategoryType() == penum.MissionCategoryType_MissionCategoryType_Daily && !SameJSTDay(mission.GetLastUpdateTime(), now) {
			threshold := int64(0)
			for _, progress := range master.GetProgresses() {
				if progress.GetThreshold() > 0 && (threshold == 0 || progress.GetThreshold() < threshold) {
					threshold = progress.GetThreshold()
				}
			}
			if threshold == 0 {
				continue
			}
			mission = &ptransaction.UserMission{MissionId: master.GetId(), CurrentThreshold: threshold, StatusType: penum.MissionStatusType_MissionStatusType_Progress}
			changed = true
		}
		var value int64
		switch master.GetMissionType() {
		case penum.MissionType_MissionType_LoveLevelAchievement:
			value = int64(catalog.Level(exp))
		case penum.MissionType_MissionType_LoveStoryEndUnlockCount:
			for _, ending := range routes.Endings {
				if slices.Contains(love.GetUnlockedLoveStoryEndIDs(), ending.ID) {
					value++
				}
			}
		case penum.MissionType_MissionType_LoginCount:
			if !SameJSTDay(mission.GetLastUpdateTime(), now) {
				value = mission.GetProgress() + 1
			}
		case penum.MissionType_MissionType_ConditionClear:
			if len(master.GetTargetIds()) == 0 {
				break
			}
			satisfied := true
			for _, conditionID := range master.GetTargetIds() {
				ok, err := loveMissionCondition(evaluator, routes, love, conditionID)
				if err != nil {
					return err
				}
				satisfied = satisfied && ok
			}
			if satisfied {
				value = 1
			}
		}
		if value > 0 {
			completed, progressed, err := reachMissionProgress(mission, master, value, now)
			if err != nil {
				return err
			}
			changed = changed || progressed
			if completed {
				mission.StatusType = penum.MissionStatusType_MissionStatusType_NotReceived
			}
		}
		if changed {
			mission.LastUpdateTime = now
			if err := opuser.UpsertUserMission(ctx, suid, mission); err != nil {
				return err
			}
			replaced := false
			for i, prior := range updated.Missions {
				if prior.GetMissionId() == mission.GetMissionId() {
					updated.Missions[i] = mission
					replaced = true
					break
				}
			}
			if !replaced {
				updated.Missions = append(updated.Missions, mission)
			}
		}
	}
	return nil
}

func LoveMissionGroups(ctx context.Context, catalog *moshikoi.Catalog) ([]*pmaster.EventMission, error) {
	ids := append(slices.Clone(catalog.Config.EventMissionIDs), catalog.Config.LimitedEventMissionIDs...)
	groups, err := opmaster.FindEventMissions(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*pmaster.EventMission, len(groups))
	for _, group := range groups {
		byID[group.GetId()] = group
	}
	ordered := make([]*pmaster.EventMission, 0, len(ids))
	for _, id := range ids {
		if byID[id] == nil {
			return nil, fmt.Errorf("love event mission group %q is absent from master data", id)
		}
		ordered = append(ordered, byID[id])
	}
	return ordered, nil
}

func loveMissionCondition(evaluator *CommerceConditionEvaluator, routes *moshikoi.Routes, love *ptransaction.UserLove, id string) (bool, error) {
	// The master condition payloads for TRUE-ending missions are empty. The
	// explicit mapping belongs with the event's route rules, not generic logic.
	if endingID, ok := routes.EndingConditions[id]; ok {
		return slices.Contains(love.GetUnlockedLoveStoryEndIDs(), endingID), nil
	}
	return evaluator.Satisfied(id)
}
