package quest

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

// The current help/captures specify five; Area.DailyLimitCount still contains
// the old value three. FREE passes are a per-area daily allowance, not items.
const freeDailyPassCount int32 = 5
const towerPageSize = 5

type questCatalog struct {
	Main          []*papi.QuestAreaGroupInfo
	Daily         []*papi.QuestAreaInfo
	Contest       *papi.QuestAreaInfo
	Tower         *papi.QuestTowerAreaInfo
	SubTowers     []*papi.QuestTowerAreaInfo
	ContestScore  int64
	HasUnplayed   bool
	DailyPlayable bool
	TowerUnlocked bool
}

// loadQuestCatalog runs inside the caller's snapshot transaction. Only daily
// counter rollover and stale current-area pointers are reconciled; listing
// never completes a live, spends a pass, or grants a clear reward.
func loadQuestCatalog(ctx context.Context, suid string, now int64, updated *papi.UpdatedData) (*questCatalog, error) {
	areas, err := opmaster.FindAllAreas(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := opmaster.FindAllAreaGroups(ctx)
	if err != nil {
		return nil, err
	}
	quests, err := opmaster.FindAllQuests(ctx)
	if err != nil {
		return nil, err
	}
	userAreas, err := opuser.FindAllUserAreas(ctx, suid)
	if err != nil {
		return nil, err
	}
	userQuests, err := opuser.FindAllUserQuests(ctx, suid)
	if err != nil {
		return nil, err
	}
	if err := viber.ReconcileQuestDailyCounters(ctx, suid, now, areas, userAreas, userQuests, updated); err != nil {
		return nil, err
	}
	byQuest := make(map[string]*pmaster.Quest, len(quests))
	byUserQuest := make(map[string]*ptransaction.UserQuest, len(userQuests))
	byUserArea := make(map[string]*ptransaction.UserArea, len(userAreas))
	for _, q := range quests {
		byQuest[q.GetId()] = q
	}
	for _, q := range userQuests {
		byUserQuest[q.GetQuestId()] = q
	}
	for _, area := range userAreas {
		byUserArea[area.GetAreaId()] = area
	}
	cleared := func(id string) bool { return byUserQuest[id].GetLastClearedTime() > 0 }
	areaCleared := func(area *pmaster.Area) bool {
		if len(area.GetQuestIds()) == 0 {
			return false
		}
		if byUserArea[area.GetId()].GetClearQuestCount() >= int32(len(area.GetQuestIds())) {
			return true
		}
		for _, id := range area.GetQuestIds() {
			if !cleared(id) {
				return false
			}
		}
		return true
	}
	sort.Slice(areas, func(i, j int) bool {
		if areas[i].GetOrder() == areas[j].GetOrder() {
			return areas[i].GetId() < areas[j].GetId()
		}
		return areas[i].GetOrder() < areas[j].GetOrder()
	})
	sort.Slice(groups, func(i, j int) bool { return groups[i].GetOrder() < groups[j].GetOrder() })
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
	var conditionIDs []string
	for _, area := range areas {
		if !questAreaSupported(area) {
			continue
		}
		conditionIDs = append(conditionIDs, area.GetViewConditionId(), area.GetUnlockConditionId())
		for _, id := range area.GetQuestIds() {
			q := byQuest[id]
			conditionIDs = append(conditionIDs, q.GetViewConditionId(), q.GetUnlockConditionId())
		}
	}
	if err := evaluator.PrimeConditionIDs(conditionIDs); err != nil {
		return nil, err
	}
	result := &questCatalog{Main: []*papi.QuestAreaGroupInfo{}, Daily: []*papi.QuestAreaInfo{}, SubTowers: []*papi.QuestTowerAreaInfo{}}
	mainInfos := map[string]*papi.QuestAreaInfo{}
	groupOrder := map[string]int32{}
	for _, group := range groups {
		groupOrder[group.GetId()] = group.GetOrder()
	}
	var reachedGroup int32
	var currentMain string
	allPreviousCleared := true
	for _, area := range areas {
		if !questAreaSupported(area) {
			continue
		}
		visible, err := evaluator.Satisfied(area.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		unlocked, err := evaluator.Satisfied(area.GetUnlockConditionId())
		if err != nil {
			return nil, err
		}
		info := &papi.QuestAreaInfo{AreaId: area.GetId(), Unlocked: unlocked, Quests: []*papi.QuestInfo{}}
		isTower := area.GetType() == penum.AreaType_AreaType_Tower || area.GetType() == penum.AreaType_AreaType_SubTower
		currentIndex := -1
		if isTower {
			for i, id := range area.GetQuestIds() {
				if !cleared(id) {
					currentIndex = i
					break
				}
			}
		}
		if area.GetType() == penum.AreaType_AreaType_Main {
			if allPreviousCleared {
				reachedGroup = max(reachedGroup, groupOrder[area.GetAreaGroupId()])
				currentMain = area.GetId()
			}
			info.Unlocked = allPreviousCleared && unlocked || areaCleared(area)
			allPreviousCleared = allPreviousCleared && areaCleared(area)
		}
		visibleLimit := len(area.GetQuestIds())
		if isTower && currentIndex >= 0 {
			visibleLimit = min(visibleLimit, (currentIndex/towerPageSize+1)*towerPageSize)
		}
		for i, id := range area.GetQuestIds() {
			if i >= visibleLimit {
				break
			}
			q := byQuest[id]
			if q == nil {
				return nil, viber.FailedPreconditionf("area %q references missing quest %q", area.GetId(), id)
			}
			show, err := evaluator.Satisfied(q.GetViewConditionId())
			if err != nil {
				return nil, err
			}
			if !show {
				continue
			}
			canPlay, err := evaluator.Satisfied(q.GetUnlockConditionId())
			if err != nil {
				return nil, err
			}
			row := &papi.QuestInfo{QuestId: id, Cleared: cleared(id), IsPlayable: info.GetUnlocked() && canPlay}
			switch area.GetType() {
			case penum.AreaType_AreaType_Main:
				row.IsPlayable = row.IsPlayable && !row.GetCleared()
			case penum.AreaType_AreaType_Tower, penum.AreaType_AreaType_SubTower:
				row.IsPlayable = row.IsPlayable && i == currentIndex && !row.GetCleared()
			case penum.AreaType_AreaType_Daily, penum.AreaType_AreaType_Contest:
				stored := byUserQuest[id]
				row.HighestScore, row.RankType = stored.GetHighestScore(), stored.GetHighestScoreRank()
				row.HighestRank = int32(min(int64(math.MaxInt32), max(int64(0), stored.GetHighestRank())))
				if area.GetType() == penum.AreaType_AreaType_Daily && row.IsPlayable {
					used := int32(0)
					if state := byUserArea[area.GetId()]; viber.SameJSTDay(state.GetLastClearedTime(), now) {
						used = max(0, state.GetDailyClearCount())
					}
					row.PlayableCount = max(0, freeDailyPassCount-used)
					// Top's notification tracks remaining FREE attempts. Individual
					// quests remain eligible when those are exhausted (paid passes).
					result.DailyPlayable = result.DailyPlayable || row.PlayableCount > 0
				}
				if area.GetType() == penum.AreaType_AreaType_Contest {
					if row.HighestScore < 0 || result.ContestScore > math.MaxInt64-row.HighestScore {
						return nil, viber.FailedPreconditionf("contest score is invalid")
					}
					result.ContestScore += row.HighestScore
					result.HasUnplayed = result.HasUnplayed || row.IsPlayable && !row.Cleared
				}
			}
			info.Quests = append(info.Quests, row)
		}
		switch area.GetType() {
		case penum.AreaType_AreaType_Main:
			mainInfos[area.GetId()] = info
		case penum.AreaType_AreaType_Tutorial:
			if !areaCleared(area) {
				mainInfos[area.GetId()] = info
			}
		case penum.AreaType_AreaType_Daily:
			result.Daily = append(result.Daily, info)
		case penum.AreaType_AreaType_Contest:
			result.Contest = info
		case penum.AreaType_AreaType_Tower, penum.AreaType_AreaType_SubTower:
			currentID := ""
			if len(area.GetQuestIds()) > 0 {
				index := currentIndex
				if index < 0 {
					index = len(area.GetQuestIds()) - 1
				}
				currentID = area.GetQuestIds()[index]
			}
			tower := &papi.QuestTowerAreaInfo{Area: info, CurrentTowerQuestId: currentID}
			if area.GetType() == penum.AreaType_AreaType_Tower {
				result.Tower = tower
			} else {
				result.SubTowers = append(result.SubTowers, tower)
			}
		}
	}
	for _, group := range groups {
		entry := &papi.QuestAreaGroupInfo{AreaGroupId: group.GetId(), Areas: []*papi.QuestAreaInfo{}}
		for _, id := range group.GetAreaIds() {
			area := mainInfos[id]
			if area == nil {
				continue
			}
			// Tutorial is a separate non-progression group in the captured menus.
			if group.GetOrder() > reachedGroup && !isTutorialArea(areas, id) {
				continue
			}
			entry.Areas = append(entry.Areas, area)
		}
		if len(entry.Areas) > 0 {
			result.Main = append(result.Main, entry)
		}
	}
	lock, err := opmaster.FindFunctionLock(ctx, penum.FunctionLockType_FunctionLockType_QuestSubTower)
	if err != nil {
		return nil, err
	}
	result.TowerUnlocked, err = evaluator.Satisfied(lock.GetUnlockConditionId())
	if err != nil {
		return nil, err
	}
	current, err := opuser.FindUserCurrentQuestArea(ctx, suid)
	if err != nil {
		return nil, err
	}
	if currentMain != "" && current.CurrentMainAreaId != currentMain || result.Tower != nil && current.CurrentTowerQuestId != result.Tower.GetCurrentTowerQuestId() {
		if currentMain != "" {
			current.CurrentMainAreaId = currentMain
		}
		if result.Tower != nil {
			current.CurrentTowerQuestId = result.Tower.GetCurrentTowerQuestId()
		}
		if err := opuser.UpsertUserCurrentQuestArea(ctx, current); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func questAreaSupported(area *pmaster.Area) bool {
	switch area.GetType() {
	case penum.AreaType_AreaType_Main, penum.AreaType_AreaType_Tower, penum.AreaType_AreaType_SubTower, penum.AreaType_AreaType_Contest, penum.AreaType_AreaType_Tutorial:
		return true
	case penum.AreaType_AreaType_Daily:
		return area.GetDailySpecialTicketItemId() == ""
	}
	return false
}

func isTutorialArea(areas []*pmaster.Area, id string) bool {
	for _, area := range areas {
		if area.GetId() == id {
			return area.GetType() == penum.AreaType_AreaType_Tutorial
		}
	}
	return false
}

func rankingUser(ctx context.Context, suid string) (*ptransaction.User, *ptransaction.UserPublic, error) {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, nil, err
	}
	public, err := opuser.FindUserPublic(ctx, suid)
	return user, public, err
}

func questNow() int64 { return time.Now().UnixMilli() }
