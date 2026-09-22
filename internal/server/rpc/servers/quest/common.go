package quest

import (
	"context"
	"errors"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	masterversion "github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/makino/salary"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"time"
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

type questBulkDailyEntry struct {
	Info          *papi.QuestBulkDailyInfo
	DeckNumber    int32
	DeckName      string
	DeckPositions []*ptransaction.UserDeckPosition
	PlayCount     int32
}

// Bulk play follows the highest cleared ordinary daily live in each area and
// its latest successful formation, independent of later saved-unit edits.
// Only formation/equipment references are retained; current card and equipment
// values are prepared again when the bulk live actually runs.
func loadQuestBulkDailyEntries(ctx context.Context, suid string, catalog *questCatalog) ([]questBulkDailyEntry, error) {
	selected := make([]*papi.QuestInfo, 0, len(catalog.Daily))
	ids := make([]string, 0, len(catalog.Daily))
	for _, area := range catalog.Daily {
		var highest *papi.QuestInfo
		for _, quest := range area.GetQuests() {
			if quest.GetCleared() && quest.GetIsPlayable() && quest.GetPlayableCount() > 0 {
				highest = quest
			}
		}
		if highest != nil {
			selected = append(selected, highest)
			ids = append(ids, highest.GetQuestId())
		}
	}
	entries := make([]questBulkDailyEntry, 0, len(selected))
	if len(selected) == 0 {
		return entries, nil
	}
	replays, err := opuser.FindQuestLatestClearReplayInfos(ctx, suid, ids)
	if err != nil {
		return nil, err
	}
	if len(replays) == 0 {
		return entries, nil
	}
	for _, quest := range selected {
		replay := replays[quest.GetQuestId()]
		// God-created clears and older replays have no reliable formation.
		// Do not guess equipment from the incomplete display-only card list.
		if replay == nil || replay.DeckNumber <= 0 {
			continue
		}
		lineup := replay.DeckPositions
		if len(lineup) != 5 {
			continue
		}
		projected := make([]*papi.QuestBulkDailyDeckPositionInfo, 5)
		cards := make(map[string]bool, 5)
		valid := true
		for _, position := range lineup {
			lane, id := position.GetPosition(), position.GetCardId()
			if lane < 1 || lane > 5 || id == "" || projected[lane-1] != nil || cards[id] {
				valid = false
				break
			}
			projected[lane-1] = &papi.QuestBulkDailyDeckPositionInfo{Position: lane, CardId: id}
			cards[id] = true
		}
		if !valid {
			continue
		}
		master, err := opmaster.FindQuest(ctx, quest.GetQuestId())
		if err != nil {
			return nil, err
		}
		rank, plus := questStartScoreRank(questStartRankPatterns(master, penum.AreaType_AreaType_Daily), replay.Score)
		entries = append(entries, questBulkDailyEntry{
			Info:       &papi.QuestBulkDailyInfo{QuestId: quest.GetQuestId(), DeckPositions: projected, RankType: rank, RankPlus: plus},
			DeckNumber: replay.DeckNumber, DeckName: replay.DeckName, DeckPositions: lineup, PlayCount: quest.GetPlayableCount(),
		})
	}
	return entries, nil
}

func questBulkDailyInfos(entries []questBulkDailyEntry) []*papi.QuestBulkDailyInfo {
	infos := make([]*papi.QuestBulkDailyInfo, 0, len(entries))
	for _, entry := range entries {
		infos = append(infos, entry.Info)
	}
	return infos
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
			if q.GetClearScore() == 0 && (area.GetType() == penum.AreaType_AreaType_Main || area.GetType() == penum.AreaType_AreaType_Tower) {
				// Known single-player boss rosters are safe to preview. A missing
				// future roster must not make the entire live catalog unavailable;
				// Start performs its own unsupported-opponent preflight.
				if opponent, err := live.NPCRoster(q.GetId()); err == nil {
					row.OpponentInfo = opponent
				}
			}
			switch area.GetType() {
			case penum.AreaType_AreaType_Main:
				row.IsPlayable = row.IsPlayable && !row.GetCleared()
			case penum.AreaType_AreaType_Tower, penum.AreaType_AreaType_SubTower:
				row.IsPlayable = row.IsPlayable && i == currentIndex && !row.GetCleared()
			case penum.AreaType_AreaType_Daily, penum.AreaType_AreaType_Contest:
				stored := byUserQuest[id]
				row.HighestScore, row.RankType = stored.GetHighestScore(), stored.GetHighestScoreRank()
				if row.HighestScore > 0 {
					row.RankType, row.Plus = questStartScoreRank(questStartRankPatterns(q, area.GetType()), row.HighestScore)
				}
				if area.GetType() == penum.AreaType_AreaType_Contest {
					row.HighestRank = int32(min(int64(math.MaxInt32), max(int64(0), stored.GetHighestRank())))
				}
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

// Contest Splus uses the player-fitted recurrence with a uniform authored
// coefficient. The official coefficient varies by quest and is not in master.
// Start, menus and retained replay all use these same disclosed thresholds.
const questContestRankGrowth = 0.003

func questStartRankPatterns(q *pmaster.Quest, mode penum.AreaType) []*papi.QuestRankPattern {
	if mode != penum.AreaType_AreaType_Daily && mode != penum.AreaType_AreaType_Contest {
		// Main/tower lives use these three display thresholds even though their
		// final rankType remains Unknown and they have no score-grade rewards.
		// NPC battles intentionally use zero thresholds, as in official traces.
		return viber.LiveClearRankPatterns(q.GetClearScore())
	}
	clear := q.GetClearScore()
	if clear <= 0 {
		return nil
	}
	var result []*papi.QuestRankPattern
	ratio := 1.3
	limit := int32(20)
	if mode == penum.AreaType_AreaType_Contest {
		limit = 99
	}
	for n := int32(1); n <= limit; n++ {
		threshold := float64(clear) * float64(n+1)
		if mode == penum.AreaType_AreaType_Contest {
			ratio += .1 + questContestRankGrowth*float64(n) + 4*questContestRankGrowth*float64(n/5)
			threshold = math.Round(float64(clear) * ratio)
		}
		result = append(result, &papi.QuestRankPattern{RankType: penum.ResultRankType_ResultRankType_Splus, Plus: n, ThresholdScore: int64(min(float64(math.MaxInt64-1024), threshold))})
	}
	for _, entry := range []struct {
		rank   penum.ResultRankType
		tenths float64
	}{
		{penum.ResultRankType_ResultRankType_S, 13},
		{penum.ResultRankType_ResultRankType_A, 12},
		{penum.ResultRankType_ResultRankType_B, 11},
		{penum.ResultRankType_ResultRankType_C, 10},
	} {
		result = append(result, &papi.QuestRankPattern{RankType: entry.rank, ThresholdScore: int64(math.Floor(float64(clear) * entry.tenths / 10))})
	}
	d := int64(0)
	if mode == penum.AreaType_AreaType_Daily {
		d = clear / 1000
	}
	result = append(result, &papi.QuestRankPattern{RankType: penum.ResultRankType_ResultRankType_D, ThresholdScore: d})
	sort.Slice(result, func(i, j int) bool { return result[i].GetThresholdScore() > result[j].GetThresholdScore() })
	return result
}

func questStartScoreRank(patterns []*papi.QuestRankPattern, score int64) (penum.ResultRankType, int32) {
	for _, p := range patterns {
		if score >= p.GetThresholdScore() {
			return p.GetRankType(), p.GetPlus()
		}
	}
	if len(patterns) > 0 {
		return penum.ResultRankType_ResultRankType_D, 0
	}
	return penum.ResultRankType_ResultRankType_Unknown, 0
}

// Replays saved before non-contest display thresholds were implemented contain
// no patterns. Repair only the response projection; retain historical traces.
func questReplayRankPatterns(ctx context.Context, id string, patterns []*papi.QuestRankPattern) ([]*papi.QuestRankPattern, error) {
	if len(patterns) > 0 {
		return patterns, nil
	}
	q, err := opmaster.FindQuest(ctx, id)
	if err != nil {
		return nil, err
	}
	area, err := opmaster.FindArea(ctx, q.GetAreaId())
	if err != nil {
		return nil, err
	}
	return questStartRankPatterns(q, area.GetType()), nil
}

func reconcileQuestContestSeason(ctx context.Context, quest *ptransaction.UserQuest, now int64) error {
	seasons, err := opmaster.FindAllQuestContestRankingSeasons(ctx)
	if err != nil {
		return err
	}
	var active *pmaster.QuestContestRankingSeason
	for _, season := range seasons {
		if season.GetId() != "" && season.GetStartTime() <= now && (active == nil || season.GetStartTime() > active.GetStartTime()) {
			active = season
		}
	}
	if active == nil {
		return viber.FailedPreconditionf("contest has no active ranking season")
	}
	if quest.GetCurrentRankingSeasonID() == active.GetId() {
		return nil
	}
	// Earlier private-server results omitted the season ID. Preserve scores
	// whose timestamp proves they belong to this season; never invent a new
	// lifetime milestone or carry an older season's score into a new one.
	score, scoreTime := int64(0), int64(0)
	if quest.GetCurrentRankingSeasonID() == "" {
		if quest.GetCurrentRankingHighestScoreTime() >= active.GetStartTime() {
			score, scoreTime = quest.GetCurrentRankingHighestScore(), quest.GetCurrentRankingHighestScoreTime()
		}
		if quest.GetHighestScoreTime() >= active.GetStartTime() && quest.GetHighestScore() > score {
			score, scoreTime = quest.GetHighestScore(), quest.GetHighestScoreTime()
		}
	}
	quest.CurrentRankingSeasonID = active.GetId()
	quest.CurrentRankingHighestScore, quest.CurrentRankingHighestScoreTime = score, scoreTime
	quest.CurrentRankingHighestRank = 0
	if score > 0 {
		quest.CurrentRankingHighestRank = 1
	}
	return nil
}

// Private-server approximations, deliberately kept together for review.
// Random photo-daily candidates have no master weights: choose uniformly per
// play. Other zero-amount item rewards are optional drops (observed on contest
// clears); the master does not expose their probability. Use a private-server
// 20% chance per play, then select one candidate uniformly. Fan catch-up is
// inferred, not an official numerical formula.
const questOptionalItemDropChancePermil = 200
const questFanCatchupPermil int64 = 50
const questFanCatchupCapacityFactor int64 = 2

func startQuest(ctx context.Context, req *papi.QuestStartRequest, formation *viber.LiveFormation) (*papi.QuestStartResponse, error) {
	if req.GetQuestId() == "" || req.GetDeckNumber() <= 0 || req.GetPlayCount() <= 0 {
		return nil, viber.InvalidArgumentf("questId, deckNumber and a positive playCount are required")
	}
	if len(req.GetUserBuddyDeckPositions()) != 0 {
		return nil, viber.FailedPreconditionf("friend rental is unavailable in this private server")
	}
	var err error
	ctx, err = mcontext.WithLiveRequest(ctx)
	if err != nil {
		return nil, err
	}
	seed, started, _ := mcontext.GetLiveRequest(ctx)
	now := started.UnixMilli()
	suid := viber.GetSuid(ctx).String()
	var response *papi.QuestStartResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		q, err := opmaster.FindQuest(ctx, req.GetQuestId())
		if err != nil {
			return nil, err
		}
		area, err := opmaster.FindArea(ctx, q.GetAreaId())
		if err != nil {
			return nil, err
		}
		if !questAreaSupported(area) {
			return nil, viber.FailedPreconditionf("quest mode or time-limited daily live is unavailable")
		}
		// A real server-only revision write serializes different live starts for
		// this user, including requests without x-app-request-id. An unchanged
		// $set would be a MongoDB no-op and is not a reliable conflict guard.
		user, err := opuser.LockUserQuestStart(ctx, suid)
		if err != nil {
			return nil, err
		}
		catalog, err := loadQuestCatalog(ctx, suid, now, updated)
		if err != nil {
			return nil, err
		}
		if !questStartPlayable(catalog, q.GetId()) {
			return nil, viber.FailedPreconditionf("quest is locked, already cleared, or not the next tower stage")
		}
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		storedAreas, err := opuser.FindAllUserAreas(ctx, suid)
		if err != nil {
			return nil, err
		}
		areaState := &ptransaction.UserArea{AreaId: area.GetId()}
		for _, row := range storedAreas {
			if row.GetAreaId() == area.GetId() {
				areaState = row
				break
			}
		}
		if !viber.SameJSTDay(areaState.GetLastClearedTime(), now) {
			areaState.DailyClearCount = 0
		}
		old, err := opuser.FindUserQuest(ctx, suid, q.GetId())
		if errors.Is(err, mongo.ErrNoDocuments) {
			old, err = &ptransaction.UserQuest{QuestId: q.GetId()}, nil
		}
		if err != nil {
			return nil, err
		}
		// The top-level response carries the pre-attempt lifetime placement;
		// updatedData carries the post-attempt record for the client's comparison.
		// Placement ranks do not apply to daily, main or tower score grades.
		previousHighestRank := int32(0)
		if area.GetType() == penum.AreaType_AreaType_Contest {
			previousHighestRank = int32(min(int64(math.MaxInt32), max(int64(0), old.GetHighestRank())))
			if err := reconcileQuestContestSeason(ctx, old, now); err != nil {
				return nil, err
			}
		} else {
			// Repair non-contest placement values written by older god mode.
			old.HighestRank, old.CurrentRankingHighestRank = 0, 0
			old.CurrentRankingHighestScore, old.CurrentRankingHighestScoreTime = 0, 0
			old.CurrentRankingSeasonID = ""
		}
		if !viber.SameJSTDay(old.GetLastClearedTime(), now) {
			old.DailyClearCount = 0
		}
		passID, passAmount, err := questStartCost(ctx, suid, req, q, area, areaState, setting, now)
		if err != nil {
			return nil, err
		}
		attemptFormation := formation
		if attemptFormation == nil {
			attemptFormation, err = viber.LoadLiveFormation(ctx, suid, req.GetDeckNumber())
			if err != nil {
				return nil, err
			}
		}
		input, err := viber.PrepareLiveInput(ctx, suid, req, q, seed, attemptFormation)
		if err != nil {
			return nil, err
		}
		result, err := live.Simulate(ctx, input)
		if err != nil {
			var unsupported *live.UnsupportedError
			if errors.As(err, &unsupported) {
				return nil, status.Error(codes.Unimplemented, unsupported.Error())
			}
			return nil, err
		}
		if len(result.GetUserInfos()) == 0 || result.GetUserInfos()[0].GetUserResult() == nil {
			return nil, viber.FailedPreconditionf("live produced no player result")
		}
		score := result.GetUserInfos()[0].GetUserResult().GetScore()
		if score < 0 {
			return nil, viber.FailedPreconditionf("live score is negative")
		}
		patterns := questStartRankPatterns(q, area.GetType())
		rank, plus := questStartScoreRank(patterns, score)
		if area.GetType() != penum.AreaType_AreaType_Daily && area.GetType() != penum.AreaType_AreaType_Contest {
			rank, plus = penum.ResultRankType_ResultRankType_Unknown, 0
		}
		reward := &papi.QuestReward{}
		wasCleared, previousHigh := old.GetLastClearedTime() > 0, old.GetHighestScore()
		isBest := result.GetCleared() && score > previousHigh
		if result.GetCleared() {
			var managerBonus, goldBonus int64
			for _, card := range result.GetUserInfos()[0].GetUserDeck().GetCards() {
				bonuses := input.Cards[card.GetIndex()].Bonuses
				managerBonus += bonuses[penum.PhotoAbilityType_PhotoAbilityType_ManagerExp]
				goldBonus += bonuses[penum.PhotoAbilityType_PhotoAbilityType_Gold]
			}
			if passAmount > 0 {
				if err := viber.ConsumeExchangeResource(ctx, suid, penum.ResourceType_ResourceType_Item, passID, passAmount, updated); err != nil {
					return nil, err
				}
			}
			rng := rand.New(rand.NewPCG(seed^0x7175657374727764, seed))
			reward.ClearRewards, err = grantQuestReward(ctx, suid, q.GetClearRewardId(), req.GetPlayCount(), goldBonus, rng, updated)
			if err != nil {
				return nil, err
			}
			reward.ScoreRankRewards, err = grantQuestReward(ctx, suid, questStartRankReward(q, rank), req.GetPlayCount(), goldBonus, rng, updated)
			if err != nil {
				return nil, err
			}
			if plus > 0 {
				reward.ScoreRankAdditionalRewards, err = grantQuestReward(ctx, suid, q.GetRankAdditionalRewardId(), req.GetPlayCount()*plus, goldBonus, rng, updated)
				if err != nil {
					return nil, err
				}
			}
			if q.GetRewardManagerExp() > 0 {
				amount, err := questBoostedAmount(int64(q.GetRewardManagerExp()), managerBonus)
				if err != nil {
					return nil, err
				}
				amount, err = questRepeatedAmount(amount, req.GetPlayCount())
				if err != nil {
					return nil, err
				}
				user, err = viber.AddManagerExp(ctx, suid, amount, updated)
				if err != nil {
					return nil, err
				}
			}
			if areaState.GetDailyClearCount() > math.MaxInt32-req.GetPlayCount() || old.GetDailyClearCount() > math.MaxInt32-req.GetPlayCount() {
				return nil, viber.FailedPreconditionf("quest clear counter overflows")
			}
			areaState.DailyClearCount += req.GetPlayCount()
			old.DailyClearCount += req.GetPlayCount()
			areaState.LastClearedTime, old.LastClearedTime = now, now
			if !wasCleared {
				areaState.ClearQuestCount++
			}
			if isBest {
				old.HighestScore, old.HighestScoreTime, old.HighestScoreRank = score, now, rank
			}
			if area.GetType() == penum.AreaType_AreaType_Contest {
				if old.GetHighestRank() != 1 {
					// There is only one ranked manager. Award the master rank-1
					// bracket once; higher-numbered brackets are not separate wins.
					for _, r := range q.GetAchievedRankRewards() {
						if r.GetHighRank() > 1 || r.GetLowRank() < 1 {
							continue
						}
						resources, err := viber.ResolveRewardResources(ctx, r.GetRewardId(), 1)
						if err != nil {
							return nil, err
						}
						for _, resource := range resources {
							if _, err := viber.GrantLiveResource(ctx, suid, resource.GetResourceType(), resource.GetResourceId(), int64(resource.GetAmount()), updated); err != nil {
								return nil, err
							}
							reward.AchievedRankRewards = append(reward.AchievedRankRewards, &ptransaction.Reward{ResourceType: resource.GetResourceType(), ResourceId: resource.GetResourceId(), Amount: int64(resource.GetAmount())})
						}
					}
				}
				old.HighestRank = 1
				old.CurrentRankingHighestRank = 1
				if score > old.GetCurrentRankingHighestScore() {
					old.CurrentRankingHighestScore, old.CurrentRankingHighestScoreTime = score, now
				}
			}
			if err := opuser.UpsertUserArea(ctx, suid, areaState); err != nil {
				return nil, err
			}
			updated.Areas = append(updated.Areas, areaState)
			if err := opuser.UpsertUserQuest(ctx, suid, old); err != nil {
				return nil, err
			}
			if err := settleQuestCharacters(ctx, suid, req, q, setting, user, input, result, reward, updated); err != nil {
				return nil, err
			}
			if err := settleQuestMissions(ctx, suid, req, q, area, input, result, updated); err != nil {
				return nil, err
			}
			if area.GetType() == penum.AreaType_AreaType_Main && !wasCleared && areaState.GetClearQuestCount() == int32(len(area.GetQuestIds())) {
				user, err = opuser.FindUser(ctx, suid)
				if err != nil {
					return nil, err
				}
				user.QuestMainAreaLastClearCharacterIds = viber.LiveResultCharacterIDs(input, result)
				if err := opuser.UpsertUser(ctx, suid, user); err != nil {
					return nil, err
				}
				updated.User = user
			}
			if area.GetType() == penum.AreaType_AreaType_Main || area.GetType() == penum.AreaType_AreaType_Tower || area.GetType() == penum.AreaType_AreaType_SubTower {
				if _, err := loadQuestCatalog(ctx, suid, now, updated); err != nil {
					return nil, err
				}
			}
			if err := unlockQuestMusic(ctx, suid, q.GetMusicId(), updated); err != nil {
				return nil, err
			}
			if _, err := viber.ReconcileUserMissions(ctx, suid, now, updated); err != nil {
				return nil, err
			}
			if err := salary.RefreshSnapshot(ctx, suid, started); err != nil {
				return nil, err
			}
		} else if old.GetQuestId() != "" {
			// Failure is a replayable attempt, not a clear or a new ranking best.
			// The official protocol still initializes/re-emits its UserQuest row.
			if err := opuser.UpsertUserQuest(ctx, suid, old); err != nil {
				return nil, err
			}
		}
		updated.Quests = append(updated.Quests, old)
		compactQuestUpdates(updated)
		version, err := masterversion.GetMasterVersion(ctx)
		if err != nil {
			return nil, err
		}
		ruleset := live.RulesetVersion
		if len(result.GetUserInfos()) > 1 {
			bonus, err := live.NPCBalance(q, setting)
			if err != nil {
				return nil, err
			}
			ruleset += "/" + bonus.Version
		}
		if err := opuser.SaveQuestLiveResult(ctx, suid, q.GetId(), result, isBest,
			opuser.QuestLiveResultInfo{RankPatterns: patterns, RulesetVersion: ruleset, MasterVersion: version, DeckNumber: req.GetDeckNumber(), DeckPositions: attemptFormation.Positions}); err != nil {
			return nil, err
		}
		response = &papi.QuestStartResponse{QuestId: q.GetId(), Result: result, Reward: reward,
			RankType: rank, RankPlus: plus, RankPatterns: patterns, HighestScore: old.GetHighestScore(),
			HighestRank: previousHighestRank, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func questStartPlayable(catalog *questCatalog, id string) bool {
	var areas []*papi.QuestAreaInfo
	for _, g := range catalog.Main {
		areas = append(areas, g.GetAreas()...)
	}
	areas = append(areas, catalog.Daily...)
	areas = append(areas, catalog.Contest, catalog.Tower.GetArea())
	for _, tower := range catalog.SubTowers {
		areas = append(areas, tower.GetArea())
	}
	for _, area := range areas {
		for _, q := range area.GetQuests() {
			if q.GetQuestId() == id {
				return q.GetIsPlayable()
			}
		}
	}
	return false
}

func questStartCost(ctx context.Context, suid string, req *papi.QuestStartRequest, q *pmaster.Quest, area *pmaster.Area, state *ptransaction.UserArea, setting *pmaster.Setting, now int64) (string, int64, error) {
	count := req.GetPlayCount()
	var itemType penum.ItemType
	switch area.GetType() {
	case penum.AreaType_AreaType_Daily:
		if count > max(1, setting.GetQuestDailyTicketLimitUseCount()) {
			return "", 0, viber.InvalidArgumentf("daily playCount exceeds the master limit")
		}
		if !req.GetUseDailyTicket() {
			if count > max(0, freeDailyPassCount-state.GetDailyClearCount()) {
				return "", 0, viber.FailedPreconditionf("insufficient FREE daily plays; select owned passes")
			}
			return "", 0, nil
		}
		// Owned passes with a remaining FREE allowance are deliberately rejected:
		// its interaction with the shared daily counter is not yet validated.
		if state.GetDailyClearCount() < freeDailyPassCount {
			return "", 0, viber.FailedPreconditionf("use remaining FREE daily plays before selecting owned passes")
		}
		itemType = area.GetDailyTicketItemType()
	case penum.AreaType_AreaType_Contest:
		if count > max(1, setting.GetQuestContestTicketLimitUseCount()) {
			return "", 0, viber.InvalidArgumentf("contest playCount exceeds the master limit")
		}
		itemType = penum.ItemType_ItemType_QuestContestTicket
	default:
		if count != 1 {
			return "", 0, viber.InvalidArgumentf("this live permits exactly one play")
		}
		return "", 0, nil
	}
	if q.GetTicketAmount() <= 0 {
		return "", 0, viber.FailedPreconditionf("paid live has an invalid master ticket cost")
	}
	masters, err := opmaster.FindQuestTicketItems(ctx, itemType)
	if err != nil {
		return "", 0, err
	}
	if len(masters) != 1 {
		return "", 0, viber.FailedPreconditionf("live pass type %v has no unique master item", itemType)
	}
	id, amount := masters[0].GetId(), int64(q.GetTicketAmount())*int64(count)
	items, err := opuser.FindUserItems(ctx, suid, id)
	if err != nil {
		return "", 0, err
	}
	var available int64
	for _, item := range items {
		if item.GetItemId() == id && (item.GetExpiredTime() == 0 || item.GetExpiredTime() > now) {
			available += min(amount-available, max(0, item.GetAmount()))
			if available >= amount {
				return id, amount, nil
			}
		}
	}
	return "", 0, status.Error(codes.ResourceExhausted, "insufficient unexpired live passes")
}

func questStartRankReward(q *pmaster.Quest, rank penum.ResultRankType) string {
	switch rank {
	case penum.ResultRankType_ResultRankType_S, penum.ResultRankType_ResultRankType_Splus:
		return q.GetRankSRewardId()
	case penum.ResultRankType_ResultRankType_A:
		return q.GetRankARewardId()
	case penum.ResultRankType_ResultRankType_B:
		return q.GetRankBRewardId()
	case penum.ResultRankType_ResultRankType_C:
		return q.GetRankCRewardId()
	case penum.ResultRankType_ResultRankType_D:
		return q.GetRankDRewardId()
	}
	return ""
}

func grantQuestReward(ctx context.Context, suid, id string, count int32, goldBonus int64, rng *rand.Rand, updated *papi.UpdatedData) ([]*ptransaction.RewardResult, error) {
	if id == "" || count <= 0 {
		return nil, nil
	}
	resources, err := viber.ParseAutoReward(id)
	if err != nil {
		return nil, err
	}
	if resources == nil {
		row, err := opmaster.FindReward(ctx, id)
		if err != nil {
			return nil, err
		}
		resources = row.GetResources()
	}
	var randomFilms, randomItems []*pmaster.RewardResource
	for _, r := range resources {
		if r == nil || r.GetAmount() < 0 {
			return nil, viber.FailedPreconditionf("reward %q contains an invalid resource amount", id)
		}
		if r.GetAmount() > 0 {
			continue
		}
		if r.GetResourceType() != penum.ResourceType_ResourceType_Item || r.GetResourceId() == "" {
			return nil, viber.FailedPreconditionf("reward %q contains an unsupported random resource", id)
		}
		if strings.HasPrefix(r.GetResourceId(), "item-photo_shooting-") {
			randomFilms = append(randomFilms, r)
		} else {
			randomItems = append(randomItems, r)
		}
	}
	var results []*ptransaction.RewardResult
	for _, r := range resources {
		if r.GetAmount() == 0 {
			continue
		}
		amount := int64(r.GetAmount())
		if r.GetResourceType() == penum.ResourceType_ResourceType_Gold {
			amount, err = questBoostedAmount(amount, goldBonus)
			if err != nil {
				return nil, err
			}
		}
		if amount > math.MaxInt64/int64(count) {
			return nil, viber.FailedPreconditionf("quest reward overflows")
		}
		rows, err := viber.GrantLiveResource(ctx, suid, r.GetResourceType(), r.GetResourceId(), amount*int64(count), updated)
		if err != nil {
			return nil, err
		}
		results = append(results, rows...)
	}
	counts := map[string]int64{}
	for i := int32(0); i < count && len(randomFilms) > 0; i++ {
		counts[randomFilms[rng.IntN(len(randomFilms))].GetResourceId()]++
	}
	for i := int32(0); i < count && len(randomItems) > 0; i++ {
		if rng.IntN(1000) < questOptionalItemDropChancePermil {
			counts[randomItems[rng.IntN(len(randomItems))].GetResourceId()]++
		}
	}
	for _, candidate := range resources {
		if candidate.GetAmount() != 0 {
			continue
		}
		if amount := counts[candidate.GetResourceId()]; amount > 0 {
			rows, err := viber.GrantLiveResource(ctx, suid, candidate.GetResourceType(), candidate.GetResourceId(), amount, updated)
			if err != nil {
				return nil, err
			}
			results = append(results, rows...)
			delete(counts, candidate.GetResourceId())
		}
	}
	return viber.AggregateRewardResults(results)
}

func settleQuestCharacters(ctx context.Context, suid string, req *papi.QuestStartRequest, q *pmaster.Quest, setting *pmaster.Setting, user *ptransaction.User, input *live.Input, result *papi.LiveResult, reward *papi.QuestReward, updated *papi.UpdatedData) error {
	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return err
	}
	byID := make(map[string]*ptransaction.UserCharacter, len(characters))
	for _, character := range characters {
		byID[character.GetCharacterId()] = character
	}
	ids := viber.LiveResultCharacterIDs(input, result)
	fanGains := map[string]int64{}
	for _, id := range ids {
		character := byID[id]
		if character == nil {
			return viber.FailedPreconditionf("live character %q is not owned", id)
		}
		// Inferred catch-up policy: earn 5% of the gap to twice the venue
		// capacity per clear, rounded down with minimum one. Only light fans
		// increase, repeated character identities receive the gain once, and
		// accounts already above the target never lose fans or gain more.
		target := int64(q.GetMaxCapacity()) * questFanCatchupCapacityFactor
		total := int64(character.GetLightFanAmount()) + int64(character.GetMiddleFanAmount()) + int64(character.GetHeavyFanAmount())
		var gained int64
		for i := int32(0); i < req.GetPlayCount() && total < target; i++ {
			n := min(target-total, max(int64(1), (target-total)*questFanCatchupPermil/1000))
			gained += n
			total += n
		}
		if gained > int64(math.MaxInt32-character.GetLightFanAmount()) {
			return viber.FailedPreconditionf("fan amount overflows")
		}
		if gained > 0 {
			character.LightFanAmount += int32(gained)
			candidate := int64(character.GetLightFanAmount())*int64(setting.GetLiveLightFanAmountWeightPermil())/1000 + int64(character.GetMiddleFanAmount())*int64(setting.GetLiveMiddleFanAmountWeightPermil())/1000 + int64(character.GetHeavyFanAmount())*int64(setting.GetLiveHeavyFanAmountWeightPermil())/1000
			candidate = max(int64(setting.GetLiveMinimumGuaranteedAudienceAmount()), candidate)
			if candidate < 0 || candidate > math.MaxInt32 {
				return viber.FailedPreconditionf("live audience candidate amount overflows")
			}
			character.AudienceCandidateAmount = int32(candidate)
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return err
			}
		}
		fanGains[id] = gained
		updated.Characters = append(updated.Characters, character)
	}
	var cardLevels []*pmaster.CardLevel
	if q.GetRewardCardExp() > 0 {
		cardLevels, err = opmaster.FindAllCardLevels(ctx)
		if err != nil {
			return err
		}
	}
	for _, card := range result.GetUserInfos()[0].GetUserDeck().GetCards() {
		gain := fanGains[input.Cards[card.GetIndex()].CharacterID]
		var exp int32
		var truncated bool
		if q.GetRewardCardExp() > 0 {
			amount, err := questBoostedAmount(int64(q.GetRewardCardExp()), input.Cards[card.GetIndex()].Bonuses[penum.PhotoAbilityType_PhotoAbilityType_CardExp])
			if err != nil {
				return err
			}
			amount, err = questRepeatedAmount(amount, req.GetPlayCount())
			if err != nil {
				return err
			}
			exp, truncated, err = grantQuestCardExp(ctx, suid, card.GetCardId(), amount, user, cardLevels, updated)
			if err != nil {
				return err
			}
		}
		if gain > 0 || q.GetRewardCardExp() > 0 {
			reward.RewardCards = append(reward.RewardCards, &papi.QuestRewardCard{CardId: card.GetCardId(), FunAmount: gain, Exp: exp, IsExpTruncated: truncated})
		}
	}
	if q.GetRewardCardExp() > 0 {
		if _, err := viber.ReconcileCardSupportLevel(ctx, suid, user, updated); err != nil {
			return err
		}
	}
	return viber.GrantLiveMastery(ctx, suid, q.GetMusicId(), ids, req.GetPlayCount(), reward, updated)
}

func grantQuestCardExp(ctx context.Context, suid, id string, amount int64, user *ptransaction.User, levels []*pmaster.CardLevel, updated *papi.UpdatedData) (int32, bool, error) {
	card, err := opuser.FindUserCard(ctx, suid, id)
	if err != nil {
		return 0, false, err
	}
	// Assistance forbids direct training; do not silently alter the preserved
	// base level that RemoveSupport must restore. This live-EXP policy is inferred.
	if card.GetSupported() {
		return 0, true, nil
	}
	rarity, err := opmaster.FindCardRarity(ctx, int64(card.GetRarity()))
	if err != nil {
		return 0, false, err
	}
	manager, err := opmaster.FindManagerLevel(ctx, user.GetManagerLevel())
	if err != nil {
		return 0, false, err
	}
	limit := rarity.GetLevelLimit()
	if card.GetRank() > 0 {
		rank, err := opmaster.FindCardRank(ctx, card.GetRank())
		if err != nil {
			return 0, false, err
		}
		limit += rank.GetLevelLimitUpAmount()
	}
	limit = min(limit, manager.GetMaxCardLevel())
	var maxExp int64
	for _, row := range levels {
		if row.GetLevel() == limit {
			maxExp = row.GetRequiredExp()
			break
		}
	}
	if maxExp <= card.GetTotalExp() {
		return 0, true, nil
	}
	gain := min(amount, maxExp-card.GetTotalExp(), int64(math.MaxInt32))
	card.TotalExp += gain
	level := card.GetBaseLevel()
	for _, row := range levels {
		if row.GetLevel() <= limit && row.GetRequiredExp() <= card.GetTotalExp() {
			level = max(level, row.GetLevel())
		}
	}
	if level != card.GetBaseLevel() {
		master, err := opmaster.FindCard(ctx, id)
		if err != nil {
			return 0, false, err
		}
		previousLevel := card.GetBaseLevel()
		if err := usercard.ApplySupportLevel(ctx, master, card, level); err != nil {
			return 0, false, err
		}
		card.BaseLevel = level
		card.BaseVocal, card.BaseDance, card.BaseVisual, card.BaseStamina = card.Vocal, card.Dance, card.Visual, card.Stamina
		card.BasePhotoEquipableCount = card.PhotoEquipableCount
		card.BaseSkillId1, card.BaseSkillId2, card.BaseSkillId3, card.BaseSkillId4 = card.SkillId1, card.SkillId2, card.SkillId3, card.SkillId4
		card.BaseLiveAbilityId, card.BaseActivityAbilityId = card.LiveAbilityId, card.ActivityAbilityId
		stories, _, err := viber.UpdateCardStoryUnlocks(ctx, suid, master, level, card.GetEvolutionLevel())
		if err != nil {
			return 0, false, err
		}
		updated.Stories = append(updated.Stories, stories...)
		missions, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_CardLevelUpCount, int64(level-previousLevel), id, master.GetCharacterId())
		if err != nil {
			return 0, false, err
		}
		putQuestMissions(updated, missions)
	}
	if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
		return 0, false, err
	}
	updated.Cards = append(updated.Cards, card)
	if card.GetBaseLevel() > 1 {
		master, err := opmaster.FindCard(ctx, id)
		if err != nil {
			return 0, false, err
		}
		missions, err := viber.ReachCharacterCardTrainingMissions(ctx, suid, master.GetCharacterId())
		if err != nil {
			return 0, false, err
		}
		putQuestMissions(updated, missions)
	}
	return int32(gain), gain < amount, nil
}

func questBoostedAmount(amount, bonusPermil int64) (int64, error) {
	if bonusPermil < 0 || bonusPermil > math.MaxInt64-1000 || amount < 0 || amount > math.MaxInt64/(1000+bonusPermil) {
		return 0, viber.FailedPreconditionf("live reward bonus overflows")
	}
	return amount * (1000 + bonusPermil) / 1000, nil
}

func questRepeatedAmount(amount int64, count int32) (int64, error) {
	if amount < 0 || count <= 0 || amount > math.MaxInt64/int64(count) {
		return 0, viber.FailedPreconditionf("repeated live reward overflows")
	}
	return amount * int64(count), nil
}

func settleQuestMissions(ctx context.Context, suid string, req *papi.QuestStartRequest, q *pmaster.Quest, area *pmaster.Area, input *live.Input, result *papi.LiveResult, updated *papi.UpdatedData) error {
	ids := viber.LiveResultCharacterIDs(input, result)
	targets := append([]string{q.GetId(), area.GetId(), q.GetMusicId()}, ids...)
	mode := map[penum.AreaType]penum.MissionType{
		penum.AreaType_AreaType_Main:     penum.MissionType_MissionType_QuestMainClearCount,
		penum.AreaType_AreaType_Tutorial: penum.MissionType_MissionType_QuestMainClearCount,
		penum.AreaType_AreaType_Daily:    penum.MissionType_MissionType_QuestDailyClearCount,
		penum.AreaType_AreaType_Contest:  penum.MissionType_MissionType_QuestContestClearCount,
		penum.AreaType_AreaType_Tower:    penum.MissionType_MissionType_QuestTowerClearCount,
		penum.AreaType_AreaType_SubTower: penum.MissionType_MissionType_QuestSubTowerClearCount,
	}[area.GetType()]
	for _, typ := range []penum.MissionType{penum.MissionType_MissionType_QuestAllClearCount, mode, penum.MissionType_MissionType_CharacterQuestAllClearCount} {
		if typ == 0 {
			continue
		}
		missions, err := viber.AdvanceUserMissions(ctx, suid, typ, int64(req.GetPlayCount()), targets...)
		if err != nil {
			return err
		}
		putQuestMissions(updated, missions)
	}
	score := result.GetUserInfos()[0].GetUserResult().GetScore()
	missions, err := viber.ReachUserMissions(ctx, suid, penum.MissionType_MissionType_QuestAllScoreAchievement, score, targets...)
	if err != nil {
		return err
	}
	putQuestMissions(updated, missions)
	if area.GetType() == penum.AreaType_AreaType_Contest {
		missions, err = viber.ReachUserMissions(ctx, suid, penum.MissionType_MissionType_CharacterQuestContestScoreAchievement, score, targets...)
		if err != nil {
			return err
		}
		putQuestMissions(updated, missions)
		rows, err := opuser.FindAllUserQuests(ctx, suid)
		if err != nil {
			return err
		}
		var total int64
		for _, row := range rows {
			if slices.Contains(area.GetQuestIds(), row.GetQuestId()) {
				if row.GetHighestScore() > math.MaxInt64-total {
					return viber.FailedPreconditionf("contest total score overflows")
				}
				total += max(int64(0), row.GetHighestScore())
			}
		}
		missions, err = viber.ReachUserMissions(ctx, suid, penum.MissionType_MissionType_ContestTotalScoreAchievement, total)
		if err != nil {
			return err
		}
		putQuestMissions(updated, missions)
	}
	achievement := map[penum.AreaType]penum.MissionType{
		penum.AreaType_AreaType_Main:     penum.MissionType_MissionType_AreaAllMainQuestClearCountAchievement,
		penum.AreaType_AreaType_Tower:    penum.MissionType_MissionType_AreaAllTowerQuestClearCountAchievement,
		penum.AreaType_AreaType_SubTower: penum.MissionType_MissionType_AreaSubTowerQuestClearCountAchievement,
	}[area.GetType()]
	if achievement != 0 {
		masters, err := opmaster.FindAllAreas(ctx)
		if err != nil {
			return err
		}
		rows, err := opuser.FindAllUserAreas(ctx, suid)
		if err != nil {
			return err
		}
		clears := make(map[string]int32, len(rows))
		for _, row := range rows {
			clears[row.GetAreaId()] = row.GetClearQuestCount()
		}
		var total int64
		for _, master := range masters {
			if master.GetType() == area.GetType() {
				if area.GetType() == penum.AreaType_AreaType_SubTower && master.GetId() != area.GetId() {
					continue
				}
				total += int64(min(int32(len(master.GetQuestIds())), max(0, clears[master.GetId()])))
			}
		}
		if total > 0 {
			missions, err := viber.ReachUserMissions(ctx, suid, achievement, total, area.GetId())
			if err != nil {
				return err
			}
			putQuestMissions(updated, missions)
		}
	}
	// Condition/function-lock missions are reconciled once after all progress
	// has been written, rather than per reward or card.
	return nil
}

func unlockQuestMusic(ctx context.Context, suid, musicID string, updated *papi.UpdatedData) error {
	if musicID == "" {
		return nil
	}
	rows, err := opuser.FindAllUserMusics(ctx, suid)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.GetMusicId() == musicID {
			return nil
		}
	}
	row := &ptransaction.UserMusic{MusicId: musicID}
	if err := opuser.UpsertUserMusic(ctx, suid, row); err != nil {
		return err
	}
	updated.Musics = append(updated.Musics, row)
	return nil
}

func putQuestMissions(updated *papi.UpdatedData, rows []*ptransaction.UserMission) {
	for _, row := range rows {
		viber.PutUpdatedMission(updated, row)
	}
}

// A reward may touch the same collection key several times. The response is a
// final client-cache delta, not an event log containing duplicate document keys.
func compactQuestUpdates(updated *papi.UpdatedData) {
	viber.DedupePhotoUpdatedItems(updated)
	updated.Points = lastQuestRows(updated.Points, func(r *ptransaction.UserPoint) penum.PointType { return r.GetPointType() })
	updated.Areas = lastQuestRows(updated.Areas, func(r *ptransaction.UserArea) string { return r.GetAreaId() })
	updated.Quests = lastQuestRows(updated.Quests, func(r *ptransaction.UserQuest) string { return r.GetQuestId() })
	updated.Cards = lastQuestRows(updated.Cards, func(r *ptransaction.UserCard) string { return r.GetCardId() })
	updated.Characters = lastQuestRows(updated.Characters, func(r *ptransaction.UserCharacter) string { return r.GetCharacterId() })
	updated.Stories = lastQuestRows(updated.Stories, func(r *ptransaction.UserStory) string { return r.GetStoryId() })
	updated.Photos = lastQuestRows(updated.Photos, func(r *ptransaction.UserPhoto) string { return r.GetPhotoId() })
	updated.Missions = lastQuestRows(updated.Missions, func(r *ptransaction.UserMission) string { return r.GetMissionId() })
}

func lastQuestRows[T any, K comparable](rows []*T, key func(*T) K) []*T {
	last := make(map[K]int, len(rows))
	for i, row := range rows {
		last[key(row)] = i
	}
	result := rows[:0]
	for i, row := range rows {
		if last[key(row)] == i {
			result = append(result, row)
		}
	}
	return result
}

// mergeQuestUpdates merges document projections, not protobuf message fragments:
// singular messages replace prior state (including zero fields), repeated rows
// are concatenated and reduced to the final document for each collection key.
func mergeQuestUpdates(dst, src *papi.UpdatedData) {
	if src == nil {
		return
	}
	src.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.IsList() {
			to, from := dst.ProtoReflect().Mutable(field).List(), value.List()
			for i := 0; i < from.Len(); i++ {
				to.Append(from.Get(i))
			}
		} else {
			dst.ProtoReflect().Set(field, value)
		}
		return true
	})
	compactQuestUpdates(dst)
	dst.CharacterMusics = lastQuestRows(dst.CharacterMusics, func(r *ptransaction.UserCharacterMusic) string { return r.GetCharacterId() + ":" + r.GetMusicId() })
	dst.Musics = lastQuestRows(dst.Musics, func(r *ptransaction.UserMusic) string { return r.GetMusicId() })
}
