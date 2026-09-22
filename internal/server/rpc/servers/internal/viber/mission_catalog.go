package viber

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// ActiveMissionCatalog is the current user's master-driven mission selection.
// Company missions have their own API and are deliberately excluded.
type ActiveMissionCatalog struct {
	Missions          map[string]*pmaster.Mission
	Events            map[string]*pmaster.EventMission
	Unlocked          map[string]bool
	LoveGroups        map[string]string
	DreamGroup        string
	evaluator         *CommerceConditionEvaluator
	photos            []*ptransaction.UserPhoto
	photosLoaded      bool
	accessoryRarity   int64
	accessoriesLoaded bool
}

func LoadActiveMissionCatalog(ctx context.Context, suid string, now int64) (*ActiveMissionCatalog, error) {
	masters, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := opmaster.FindAllEventMissions(ctx)
	if err != nil {
		return nil, err
	}
	evaluator := NewCommerceConditionEvaluator(ctx, suid, now)
	conditionIDs := make([]string, 0, len(groups)*2+len(masters))
	for _, group := range groups {
		conditionIDs = append(conditionIDs, group.GetViewConditionId(), group.GetUnlockConditionId())
	}
	for _, master := range masters {
		if !isGeneralMission(master) {
			continue
		}
		conditionIDs = append(conditionIDs, master.GetViewConditionId())
		if master.GetMissionType() == penum.MissionType_MissionType_ConditionClear {
			conditionIDs = append(conditionIDs, master.GetTargetIds()...)
		}
	}
	if err := evaluator.PrimeConditionIDs(conditionIDs); err != nil {
		return nil, err
	}
	catalog := &ActiveMissionCatalog{
		Missions: make(map[string]*pmaster.Mission), Events: make(map[string]*pmaster.EventMission),
		Unlocked: make(map[string]bool), LoveGroups: make(map[string]string), evaluator: evaluator,
	}
	dreamMonthly, err := dream.MonthlyCatalog(ctx, now)
	if err != nil {
		return nil, err
	}
	catalog.DreamGroup = dreamMonthly.EventMissionID
	loves, err := opuser.FindAllUserLoves(ctx, suid)
	if err != nil {
		return nil, err
	}
	for _, love := range loves {
		for _, id := range moshikoi.SupportedLoveIDs() {
			if id != love.GetLoveId() {
				continue
			}
			loveConfig, err := moshikoi.LoadConfig(id)
			if err != nil {
				return nil, err
			}
			for _, groupID := range append(append([]string{}, loveConfig.EventMissionIDs...), loveConfig.LimitedEventMissionIDs...) {
				catalog.LoveGroups[groupID] = id
			}
		}
	}
	for _, group := range groups {
		if strings.HasPrefix(group.GetId(), "event-dream-") {
			if group.GetId() == catalog.DreamGroup {
				catalog.Events[group.GetId()] = group
				unlocked, err := evaluator.Satisfied(dream.UnlockConditionID)
				if err != nil {
					return nil, err
				}
				catalog.Unlocked[group.GetId()] = unlocked
			}
			continue
		}
		permanent := catalog.LoveGroups[group.GetId()] != ""
		// Unsupported Love series remain unavailable even when a revival's
		// calendar condition happens to be active in the current master data.
		if strings.HasPrefix(group.GetId(), "event-love-") && !permanent {
			continue
		}
		if permanent {
			// Match Love.Top: remove calendar gates, not card/other conditions.
			for _, id := range []string{group.GetViewConditionId(), group.GetUnlockConditionId()} {
				if condition := evaluator.conditions[id]; condition != nil {
					copy := PermanentLoveMissionCondition(condition)
					evaluator.PrimeConditions([]*pmaster.Condition{copy})
					delete(evaluator.results, id)
				}
			}
		}
		visible, err := evaluator.Satisfied(group.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		unlocked, err := evaluator.Satisfied(group.GetUnlockConditionId())
		if err != nil {
			return nil, err
		}
		catalog.Events[group.GetId()] = group
		catalog.Unlocked[group.GetId()] = unlocked
	}
	for _, master := range masters {
		if !isGeneralMission(master) {
			continue
		}
		if master.GetEventMissionId() != "" && !catalog.Unlocked[master.GetEventMissionId()] {
			continue
		}
		if catalog.LoveGroups[master.GetEventMissionId()] != "" || master.GetEventMissionId() == catalog.DreamGroup {
			id := master.GetViewConditionId()
			if condition := evaluator.conditions[id]; condition != nil {
				evaluator.PrimeConditions([]*pmaster.Condition{PermanentLoveMissionCondition(condition)})
				delete(evaluator.results, id)
			}
		}
		visible, err := evaluator.Satisfied(master.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		if visible && len(master.GetProgresses()) > 0 {
			catalog.Missions[master.GetId()] = master
		}
	}
	return catalog, nil
}

func isGeneralMission(master *pmaster.Mission) bool {
	// Company tasks have their own API. Photo-option unlock markers have no
	// reward sets and are not receivable tasks in the official Mission list.
	return master.GetCompanyEnjoyCharacterId() == "" &&
		master.GetCompanyTrustMissionCategoryType() == 0 &&
		master.GetCompanyEnjoyCharacterMissionCategoryType() == 0 &&
		master.GetCategoryType() != penum.MissionCategoryType_MissionCategoryType_Photo
}

// ReconcileUserMissions is called at login/home entry, before gameplay can
// advance counters. Repeated reads do not grant rewards or reset progress.
func ReconcileUserMissions(ctx context.Context, suid string, now int64, updated *papi.UpdatedData) ([]*papi.HomeEventMissionInfo, error) {
	catalog, err := LoadActiveMissionCatalog(ctx, suid, now)
	if err != nil {
		return nil, err
	}
	if err := catalog.Reconcile(ctx, suid, now, updated); err != nil {
		return nil, err
	}
	infos := []*papi.HomeEventMissionInfo{}
	for id, group := range catalog.Events {
		// Feature-specific events (Love, Dream, Tour...) expose their tasks
		// inside that feature, not as campaign entrances on the main home.
		if group.GetEventMissionType() != penum.EventMissionType_EventMissionType_Campaign || id == catalog.DreamGroup {
			continue
		}
		infos = append(infos, &papi.HomeEventMissionInfo{Id: id, Unlocked: catalog.Unlocked[id], Order: group.GetOrder()})
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].GetOrder() == infos[j].GetOrder() {
			return infos[i].GetId() < infos[j].GetId()
		}
		return infos[i].GetOrder() < infos[j].GetOrder()
	})
	return infos, nil
}

func (c *ActiveMissionCatalog) Reconcile(ctx context.Context, suid string, now int64, updated *papi.UpdatedData) error {
	if _, err := dream.ReconcileMonthly(ctx, suid, now, updated); err != nil {
		return err
	}
	loveIDs := map[string]bool{}
	for _, loveID := range c.LoveGroups {
		loveIDs[loveID] = true
	}
	for loveID := range loveIDs {
		state, err := moshikoi.LoadState(ctx, suid, loveID)
		if err != nil {
			return err
		}
		if err := ReconcileLoveMissions(ctx, suid, loveID, state.Exp, updated); err != nil {
			return err
		}
	}
	ids := make([]string, 0, len(c.Missions))
	for id := range c.Missions {
		ids = append(ids, id)
	}
	rows, err := opuser.FindUserMissionsByIds(ctx, suid, ids)
	if err != nil {
		return err
	}
	byID := make(map[string]*ptransaction.UserMission, len(rows))
	for _, row := range rows {
		byID[row.GetMissionId()] = row
	}
	total, err := opuser.FindUserTotalCount(ctx, suid)
	if err != nil {
		return err
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return err
	}
	changed := map[string]*ptransaction.UserMission{}
	completed := []string{}
	for id, master := range c.Missions {
		if c.LoveGroups[master.GetEventMissionId()] != "" || master.GetEventMissionId() == c.DreamGroup {
			continue
		}
		row := byID[id]
		if row == nil || missionPeriodExpired(master.GetCategoryType(), row.GetLastUpdateTime(), now) {
			threshold, ok := firstCompanyMissionThreshold(master.GetProgresses())
			if !ok {
				continue
			}
			row = &ptransaction.UserMission{MissionId: id, CurrentThreshold: threshold, StatusType: penum.MissionStatusType_MissionStatusType_Progress}
			byID[id], changed[id] = row, row
		}
		if !missionCanProgress(row) {
			continue
		}
		value, err := c.currentValue(master, row, total, user, now)
		if err != nil {
			return err
		}
		if value > row.GetProgress() {
			done, didChange, err := reachMissionProgress(row, master, value, now)
			if err != nil {
				return err
			}
			if didChange {
				changed[id] = row
			}
			if done {
				completed = append(completed, id)
			}
		}
		if changed[id] != nil {
			row.LastUpdateTime = now
		}
	}
	for len(completed) > 0 {
		next := []string{}
		for _, id := range completed {
			row := byID[id]
			row.StatusType = penum.MissionStatusType_MissionStatusType_Clear
			for parentID, master := range c.Missions {
				parent := byID[parentID]
				if parent == nil || master.GetMissionType() != penum.MissionType_MissionType_MissionClearCount || !containsString(master.GetTargetIds(), id) {
					continue
				}
				done, didChange, err := addMissionProgress(parent, master, 1, now)
				if err != nil {
					return err
				}
				if didChange {
					changed[parentID] = parent
				}
				if done {
					// Mark completion immediately: another child in this same
					// wave may also progress this parent before its queued turn.
					// It must not enqueue the same newly cleared tier twice.
					parent.StatusType = penum.MissionStatusType_MissionStatusType_Clear
					next = append(next, parentID)
				}
			}
		}
		completed = next
	}
	changedRows := make([]*ptransaction.UserMission, 0, len(changed))
	for _, row := range changed {
		changedRows = append(changedRows, row)
		PutUpdatedMission(updated, row)
	}
	return opuser.UpsertManyUserMissions(ctx, suid, changedRows)
}

func (c *ActiveMissionCatalog) currentValue(master *pmaster.Mission, row *ptransaction.UserMission, total *ptransaction.UserTotalCount, user *ptransaction.User, now int64) (int64, error) {
	switch master.GetMissionType() {
	case penum.MissionType_MissionType_LoginCount:
		if !SameJSTDay(row.GetLastUpdateTime(), now) {
			return row.GetProgress() + 1, nil
		}
	case penum.MissionType_MissionType_LoginTotalCount:
		return total.GetLoginCount(), nil
	case penum.MissionType_MissionType_PointGoldTotalAmount:
		return total.GetPointGoldAmount(), nil
	case penum.MissionType_MissionType_MarketExchangeTotalCount:
		return total.GetMarketExchangeCount(), nil
	case penum.MissionType_MissionType_PhotoShootTotalCount:
		return total.GetPhotoShootCount(), nil
	case penum.MissionType_MissionType_PhotoRetouchTotalCount:
		return total.GetPhotoRetouchCount(), nil
	case penum.MissionType_MissionType_DreamAreaGenerateTotalCount:
		return total.GetDreamAreaGenerateCount(), nil
	case penum.MissionType_MissionType_DreamQuestClearTotalCount, penum.MissionType_MissionType_DreamQuestClearDifficultyLevelAchievement:
		state, err := opuser.FindUserDreamState(c.evaluator.ctx, c.evaluator.suid)
		if err != nil {
			return 0, err
		}
		if master.GetMissionType() == penum.MissionType_MissionType_DreamQuestClearTotalCount {
			return state.QuestClearCount, nil
		}
		return int64(state.MaxDifficulty), nil
	case penum.MissionType_MissionType_StaffTrainTotalCount:
		return total.GetStaffTrainCount(), nil
	case penum.MissionType_MissionType_GuildCheckInTotalCount:
		return total.GetGuildCheckInCount(), nil
	case penum.MissionType_MissionType_ForumLikeTotalCount:
		return total.GetForumLikeCount(), nil
	case penum.MissionType_MissionType_ManagerLevelAchievement:
		return int64(user.GetManagerLevel()), nil
	case penum.MissionType_MissionType_DeckMaxPowerAchievement:
		return user.GetDeckMaxOverallValue(), nil
	case penum.MissionType_MissionType_CardLevelAchievement:
		cards, err := c.evaluator.loadCards()
		if err != nil {
			return 0, err
		}
		var highest int64
		for _, card := range cards {
			if missionTargetsMatch(master.GetTargetIds(), []string{card.GetCardId()}) {
				highest = max(highest, int64(card.GetLevel()))
			}
		}
		return highest, nil
	case penum.MissionType_MissionType_ReliabilityAchievement, penum.MissionType_MissionType_ActivityLevelAchievement:
		characters, err := c.evaluator.loadCharacters()
		if err != nil {
			return 0, err
		}
		var highest int64
		for _, character := range characters {
			if !missionTargetsMatch(master.GetTargetIds(), []string{character.GetCharacterId()}) {
				continue
			}
			value := character.GetReliability()
			if master.GetMissionType() == penum.MissionType_MissionType_ActivityLevelAchievement {
				value = character.GetActivityLevel()
			}
			highest = max(highest, int64(value))
		}
		return highest, nil
	case penum.MissionType_MissionType_AccessoryRarityAchievement:
		if !c.accessoriesLoaded {
			accessories, err := opuser.FindAllUserAccessories(c.evaluator.ctx, c.evaluator.suid)
			if err != nil {
				return 0, err
			}
			masters, err := opmaster.FindAllAccessories(c.evaluator.ctx)
			if err != nil {
				return 0, err
			}
			owned := map[string]bool{}
			for _, accessory := range accessories {
				if accessory.GetAmount() > 0 {
					owned[accessory.GetAccessoryId()] = true
				}
			}
			for _, master := range masters {
				if owned[master.GetId()] {
					c.accessoryRarity = max(c.accessoryRarity, int64(master.GetRarity()))
				}
			}
			c.accessoriesLoaded = true
		}
		return c.accessoryRarity, nil
	case penum.MissionType_MissionType_PhotoLevelAchievement:
		if !c.photosLoaded {
			var err error
			c.photos, err = opuser.FindAllUserPhotos(c.evaluator.ctx, c.evaluator.suid)
			if err != nil {
				return 0, err
			}
			c.photosLoaded = true
		}
		var highest int64
		for _, photo := range c.photos {
			highest = max(highest, int64(photo.GetLevel()))
		}
		return highest, nil
	case penum.MissionType_MissionType_ConditionClear:
		if len(master.GetTargetIds()) == 0 {
			return 0, nil
		}
		for _, id := range master.GetTargetIds() {
			ok, err := c.evaluator.Satisfied(id)
			if err != nil || !ok {
				return 0, err
			}
		}
		return 1, nil
	}
	return 0, nil
}

func missionPeriodExpired(category penum.MissionCategoryType, before, now int64) bool {
	if before <= 0 {
		return false
	} // Old fixtures/rows predate period timestamps.
	switch category {
	case penum.MissionCategoryType_MissionCategoryType_Daily:
		return !SameJSTDay(before, now)
	case penum.MissionCategoryType_MissionCategoryType_Weekly:
		jst := time.FixedZone("JST", 9*60*60)
		weekStart := func(millis int64) time.Time {
			t := time.UnixMilli(millis).In(jst)
			return time.Date(t.Year(), t.Month(), t.Day()-(int(t.Weekday())+6)%7, 0, 0, 0, 0, jst)
		}
		return !weekStart(before).Equal(weekStart(now))
	}
	return false
}

func PutUpdatedMission(updated *papi.UpdatedData, row *ptransaction.UserMission) {
	for i, prior := range updated.Missions {
		if prior.GetMissionId() == row.GetMissionId() {
			updated.Missions[i] = row
			return
		}
	}
	updated.Missions = append(updated.Missions, row)
}
