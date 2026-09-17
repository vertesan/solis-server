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
	"math/big"
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

// questDeckCharacterAssets is shared by asset prefetch and live preparation:
// both must choose the same owned replacement and exactly the same appearance.
// Supplied master characters avoid duplicate DB reads in the Start adapter.
func questDeckCharacterAssets(ctx context.Context, positions []*ptransaction.UserDeckPosition, originalIDs []string, characters []*ptransaction.UserCharacter, masters map[string]*pmaster.Character) ([]*papi.LiveCharacterAssetInfo, error) {
	if len(positions) != len(originalIDs) {
		return nil, viber.FailedPreconditionf("live appearance inputs differ in size")
	}
	owned := make(map[string]*ptransaction.UserCharacter, len(characters))
	ordered := make([]string, 0, len(characters))
	for _, character := range characters {
		owned[character.GetCharacterId()] = character
		ordered = append(ordered, character.GetCharacterId())
	}
	sort.Strings(ordered)
	reserved, used := map[string]bool{}, map[string]bool{}
	for _, id := range originalIDs {
		reserved[id] = true
	}
	assets := make([]*papi.LiveCharacterAssetInfo, 0, len(positions))
	for index, position := range positions {
		id, costumeID, hairID := originalIDs[index], position.GetCostumeId(), position.GetHairId()
		if used[id] {
			// Replacement selection is an authored deterministic rule. Never pick
			// an unowned idol or steal a later original idol's display identity.
			id, costumeID, hairID = "", "", ""
			for _, candidate := range ordered {
				if !reserved[candidate] && !used[candidate] {
					id = candidate
					break
				}
			}
		}
		character := owned[id]
		if character == nil {
			return nil, viber.FailedPreconditionf("not enough owned idols for the live appearance")
		}
		asset, err := questCharacterAsset(ctx, character, masters[id], costumeID, hairID)
		if err != nil {
			return nil, err
		}
		used[id] = true
		assets = append(assets, asset)
	}
	return assets, nil
}

// Start and asset prefetch share extra performer selection. Extra performers
// have no cards and never participate in audience or score calculations.
func prepareExtraLiveAppearances(ctx context.Context, suid string, music *pmaster.Music, characters []*ptransaction.UserCharacter, masters map[string]*pmaster.Character, displayed []*papi.LiveCharacterAssetInfo) ([]*papi.ExtraCharacterInfo, error) {
	owned := make(map[string]*ptransaction.UserCharacter, len(characters))
	ids := make([]string, 0, len(characters))
	for _, character := range characters {
		owned[character.GetCharacterId()] = character
		ids = append(ids, character.GetCharacterId())
	}
	used := make([]string, 0, len(displayed))
	for _, asset := range displayed {
		used = append(used, asset.GetCharacterId())
	}
	chosen, err := live.ExtraCharacterIDs(music.GetDisplayCharacterAmount(), ids, used)
	if err != nil {
		return nil, viber.FailedPreconditionf("%v", err)
	}
	result := make([]*papi.ExtraCharacterInfo, 0, len(chosen))
	for i, id := range chosen {
		asset, err := questCharacterAsset(ctx, owned[id], masters[id], "", "")
		if err != nil {
			return nil, err
		}
		if _, err := opuser.FindUserCostume(ctx, suid, asset.GetCostumeId()); err != nil {
			return nil, err
		}
		if _, err := opuser.FindUserHair(ctx, suid, asset.GetHairId()); err != nil {
			return nil, err
		}
		result = append(result, &papi.ExtraCharacterInfo{Position: int32(len(displayed) + i + 1), CharacterId: asset.GetCharacterId(), CostumeId: asset.GetCostumeId(), HairId: asset.GetHairId()})
	}
	return result, nil
}

func questCharacterAsset(ctx context.Context, character *ptransaction.UserCharacter, master *pmaster.Character, costumeID, hairID string) (*papi.LiveCharacterAssetInfo, error) {
	if character == nil {
		return nil, viber.FailedPreconditionf("live appearance requires an owned idol")
	}
	var err error
	if master == nil {
		master, err = opmaster.FindCharacter(ctx, character.GetCharacterId())
		if err != nil {
			return nil, err
		}
	}
	if master.GetId() != character.GetCharacterId() {
		return nil, viber.FailedPreconditionf("live appearance character does not match master")
	}
	if costumeID == "" {
		costumeID = character.GetLiveCostumeId()
	}
	if costumeID == "" {
		costumeID = master.GetDefaultLiveCostumeId()
	}
	if costumeID == "" {
		costumeID = master.GetDefaultCostumeId()
	}
	costume, err := opmaster.FindCostume(ctx, costumeID)
	if err != nil {
		return nil, err
	}
	if costume.GetCharacterId() != character.GetCharacterId() || strings.HasPrefix(costume.GetCostumeTypeId(), "sd-") {
		return nil, viber.FailedPreconditionf("deck costume has no usable live model for this idol")
	}
	if hairID == "" {
		hairID = character.GetLiveHairId()
	}
	if hairID == "" {
		hairID = costume.GetDefaultHairId()
	}
	hair, err := opmaster.FindHair(ctx, hairID)
	if err != nil {
		return nil, err
	}
	fits := func(hair *pmaster.Hair) bool {
		return hair.GetCharacterId() == character.GetCharacterId() && !slices.Contains(hair.GetNotWearableCostumeIds(), costumeID) && (len(hair.GetWearableCostumeIds()) == 0 || slices.Contains(hair.GetWearableCostumeIds(), costumeID))
	}
	if !fits(hair) {
		// Global live hair can become incompatible with a deck-specific outfit.
		// Prefetch and Start must both use the costume's compatible default.
		hairID = costume.GetDefaultHairId()
		hair, err = opmaster.FindHair(ctx, hairID)
		if err != nil {
			return nil, err
		}
		if !fits(hair) {
			return nil, viber.FailedPreconditionf("costume default hair has no compatible live model")
		}
	}
	return &papi.LiveCharacterAssetInfo{CharacterId: character.GetCharacterId(), CostumeId: costumeID, HairId: hairID}, nil
}

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
		return nil
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

// questLiveFormation preserves lineup references, not frozen card/equipment stats.
// Bulk plays revalidate these IDs against current possessions without altering a
// saved user deck. Ordinary Start reads its deck fresh on each transaction retry.
type questLiveFormation struct {
	Name      string
	Positions []*ptransaction.UserDeckPosition
}

func loadQuestLiveFormation(ctx context.Context, suid string, number int32) (*questLiveFormation, error) {
	deck, err := opuser.FindUserDeck(ctx, suid, number)
	if err != nil {
		return nil, err
	}
	positions, err := opuser.FindUserDeckPositions(ctx, suid, number)
	if err != nil {
		return nil, err
	}
	return &questLiveFormation{Name: deck.GetName(), Positions: positions}, nil
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

func startQuest(ctx context.Context, req *papi.QuestStartRequest, formation *questLiveFormation) (*papi.QuestStartResponse, error) {
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
			attemptFormation, err = loadQuestLiveFormation(ctx, suid, req.GetDeckNumber())
			if err != nil {
				return nil, err
			}
		}
		input, err := prepareLiveInput(ctx, suid, req, q, seed, attemptFormation)
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
							if _, err := grantQuestResource(ctx, suid, resource.GetResourceType(), resource.GetResourceId(), int64(resource.GetAmount()), updated); err != nil {
								return nil, err
							}
							reward.AchievedRankRewards = append(reward.AchievedRankRewards, &ptransaction.Reward{ResourceType: resource.GetResourceType(), ResourceId: resource.GetResourceId(), Amount: int64(resource.GetAmount())})
						}
					}
				}
				old.HighestRank = 1
				old.CurrentRankingHighestRank = 1
				old.CurrentRankingHighestScore, old.CurrentRankingHighestScoreTime = old.HighestScore, old.HighestScoreTime
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
				user.QuestMainAreaLastClearCharacterIds = questResultCharacterIDs(input, result)
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
		rows, err := grantQuestResource(ctx, suid, r.GetResourceType(), r.GetResourceId(), amount*int64(count), updated)
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
			rows, err := grantQuestResource(ctx, suid, candidate.GetResourceType(), candidate.GetResourceId(), amount, updated)
			if err != nil {
				return nil, err
			}
			results = append(results, rows...)
			delete(counts, candidate.GetResourceId())
		}
	}
	return viber.AggregateRewardResults(results)
}

func grantQuestResource(ctx context.Context, suid string, resourceType penum.ResourceType, id string, amount int64, updated *papi.UpdatedData) ([]*ptransaction.RewardResult, error) {
	var results []*ptransaction.RewardResult
	for amount > 0 {
		chunk := min(amount, int64(math.MaxInt32))
		if resourceType == penum.ResourceType_ResourceType_Card {
			chunk = 1
		}
		rows, err := viber.GrantGachaReward(ctx, suid, &pmaster.RewardResource{ResourceType: resourceType, ResourceId: id, Amount: int32(chunk)}, nil, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, rows...)
		updated.Missions = lastQuestRows(updated.Missions, func(row *ptransaction.UserMission) string { return row.GetMissionId() })
		amount -= chunk
	}
	return results, nil
}

func questResultCharacterIDs(input *live.Input, result *papi.LiveResult) []string {
	var ids []string
	for _, card := range result.GetUserInfos()[0].GetUserDeck().GetCards() {
		id := input.Cards[card.GetIndex()].CharacterID
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
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
	ids := questResultCharacterIDs(input, result)
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
	return grantQuestMastery(ctx, suid, q.GetMusicId(), ids, req.GetPlayCount(), reward, updated)
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

func grantQuestMastery(ctx context.Context, suid, musicID string, characters []string, count int32, reward *papi.QuestReward, updated *papi.UpdatedData) error {
	if musicID == "" {
		return nil
	}
	levels, err := opmaster.FindAllMusicMasteryLevels(ctx)
	if err != nil {
		return err
	}
	rows, err := opuser.FindUserMusicMasteries(ctx, suid, musicID)
	if err != nil {
		return err
	}
	byID := map[string]*ptransaction.UserCharacterMusic{}
	for _, row := range rows {
		byID[row.GetCharacterId()] = row
	}
	rules, err := opmaster.FindMusicMasteryRewards(ctx, musicID)
	if err != nil {
		return err
	}
	sets := map[string]string{}
	for _, rule := range rules {
		sets[rule.GetCharacterId()] = rule.GetMusicMasteryRewardSetId()
	}
	loaded := map[string][]*pmaster.MusicMasteryRewardSet{}
	for _, id := range characters {
		row := byID[id]
		if row == nil {
			row = &ptransaction.UserCharacterMusic{MusicId: musicID, CharacterId: id}
		}
		before := row.GetMasteryLevel()
		if row.GetMasteryPoint() > math.MaxInt32-count {
			return viber.FailedPreconditionf("music mastery overflows")
		}
		row.MasteryPoint += count
		for _, level := range levels {
			if row.GetMasteryPoint() >= level.GetRequiredMasteryPoint() {
				row.MasteryLevel = max(row.GetMasteryLevel(), level.GetLevel())
			}
		}
		if err := opuser.UpsertUserCharacterMusic(ctx, suid, row); err != nil {
			return err
		}
		updated.CharacterMusics = append(updated.CharacterMusics, row)
		if row.GetMasteryLevel() <= before || sets[id] == "" {
			continue
		}
		rewards, ok := loaded[sets[id]]
		if !ok {
			rewards, err = opmaster.FindMusicMasteryRewardLevels(ctx, sets[id])
			if err != nil {
				return err
			}
			loaded[sets[id]] = rewards
		}
		for _, r := range rewards {
			if r.GetLevel() <= before || r.GetLevel() > row.GetMasteryLevel() {
				continue
			}
			resources, err := viber.ResolveRewardResources(ctx, r.GetRewardId(), 1)
			if err != nil {
				return err
			}
			for _, resource := range resources {
				if _, err := grantQuestResource(ctx, suid, resource.GetResourceType(), resource.GetResourceId(), int64(resource.GetAmount()), updated); err != nil {
					return err
				}
				reward.MusicMasteryRewards = append(reward.MusicMasteryRewards, &ptransaction.Reward{ResourceType: resource.GetResourceType(), ResourceId: resource.GetResourceId(), Amount: int64(resource.GetAmount())})
			}
		}
	}
	return nil
}

func settleQuestMissions(ctx context.Context, suid string, req *papi.QuestStartRequest, q *pmaster.Quest, area *pmaster.Area, input *live.Input, result *papi.LiveResult, updated *papi.UpdatedData) error {
	ids := questResultCharacterIDs(input, result)
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

// prepareLiveInput resolves all user/master references within the caller's
// snapshot transaction. It performs no writes and no simulator-time DB lookups.
func prepareLiveInput(ctx context.Context, suid string, req *papi.QuestStartRequest, q *pmaster.Quest, seed uint64, formation *questLiveFormation) (*live.Input, error) {
	if q == nil || req == nil || req.GetDeckNumber() <= 0 {
		return nil, viber.InvalidArgumentf("a quest and saved deck are required")
	}
	if len(req.GetUserBuddyDeckPositions()) > 0 {
		return nil, status.Error(codes.Unimplemented, "friend buddy live units are not supported")
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return nil, err
	}
	input := &live.Input{Quest: q, Setting: setting, Seed: seed, Cards: map[int32]live.CardData{}, Skills: map[string]*pmaster.Skill{}, Efficacies: map[string]*pmaster.SkillEfficacy{}, Triggers: map[string]*pmaster.SkillTrigger{}, Targets: map[string]*pmaster.SkillTarget{}}
	if input.Chart, err = opmaster.FindLiveChart(ctx, q.GetMusicChartPatternId()); err != nil {
		return nil, err
	}
	sort.Slice(input.Chart, func(i, j int) bool { return input.Chart[i].GetNumber() < input.Chart[j].GetNumber() })
	if len(input.Chart) == 0 {
		return nil, viber.FailedPreconditionf("quest has no chart")
	}
	if input.Pressure, err = opmaster.FindLivePressure(ctx, q.GetQuestPressureId()); err != nil {
		return nil, err
	}
	if input.AudienceAdvantages, err = opmaster.FindLiveAudienceAdvantages(ctx, q.GetQuestAudienceAdvantageId()); err != nil {
		return nil, err
	}
	music, err := opmaster.FindMusic(ctx, q.GetMusicId())
	if err != nil {
		return nil, err
	}
	if input.ComboAdvantages, err = opmaster.FindLiveComboAdvantages(ctx, music.GetComboAdvantageId()); err != nil {
		return nil, err
	}
	if input.StaminaConsumption, err = opmaster.FindLiveStaminaConsumption(ctx); err != nil {
		return nil, err
	}
	if formation == nil {
		return nil, viber.InvalidArgumentf("live formation is required")
	}
	deck := &ptransaction.UserDeck{Number: req.GetDeckNumber(), Name: formation.Name}
	positions := formation.Positions
	if len(positions) != 5 {
		return nil, viber.FailedPreconditionf("a live unit requires five occupied positions")
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].GetPosition() < positions[j].GetPosition() })
	owned, err := opuser.FindAllUserCards(ctx, suid)
	if err != nil {
		return nil, err
	}
	ownedByID := map[string]*ptransaction.UserCard{}
	cardIDs := map[string]bool{}
	for _, c := range owned {
		ownedByID[c.GetCardId()] = c
	}
	for _, p := range positions {
		cardIDs[p.GetCardId()] = true
	}
	var npc *papi.QuestOpponentInfo
	if q.GetClearScore() == 0 {
		npc, err = live.NPCRoster(q.GetId())
		if err != nil {
			return nil, status.Error(codes.Unimplemented, err.Error())
		}
		for _, c := range npc.GetCardInfos() {
			cardIDs[c.GetCardId()] = true
		}
	}
	masters, err := opmaster.FindLiveCards(ctx, liveStringKeys(cardIDs))
	if err != nil {
		return nil, err
	}
	byCard := map[string]*pmaster.Card{}
	for _, c := range masters {
		byCard[c.GetId()] = c
	}
	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return nil, err
	}
	byCharacter := map[string]*ptransaction.UserCharacter{}
	for _, c := range characters {
		byCharacter[c.GetCharacterId()] = c
	}
	masterCharacters, err := opmaster.FindAllCharacters(ctx)
	if err != nil {
		return nil, err
	}
	byMasterCharacter := map[string]*pmaster.Character{}
	for _, c := range masterCharacters {
		byMasterCharacter[c.GetId()] = c
	}
	staffs, err := opuser.FindAllUserStaffs(ctx, suid)
	if err != nil {
		return nil, err
	}
	staffLevels, err := opmaster.FindAllStaffLevels(ctx)
	if err != nil {
		return nil, err
	}
	staffBonus := map[penum.ParameterType]int64{}
	for _, s := range staffs {
		found := s.GetLevel() == 0
		for _, level := range staffLevels {
			if s.GetParameterType() == level.GetParameterType() && s.GetLevel() == level.GetLevel() {
				staffBonus[s.GetParameterType()] = int64(level.GetAdvantage())
				found = true
				break
			}
		}
		if !found {
			return nil, viber.FailedPreconditionf("staff level is missing from master")
		}
	}
	companyLevels, err := opmaster.FindAllCompanyEnjoyCharacterLevels(ctx)
	if err != nil {
		return nil, err
	}
	global, globalPassives, equipmentBoost, err := prepareOwnedLiveAbilities(ctx, owned, q)
	if err != nil {
		return nil, err
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, err
	}
	self := &papi.LiveUserInfo{Index: 1, UserId: public.GetPublicUserId(), Name: user.GetName(), EmblemId: user.GetEmblemId(), UserDeck: &papi.LiveUserDeck{DeckName: deck.GetName()}}
	input.Users = append(input.Users, self)
	seenCards, seenPhotos := map[string]bool{}, map[string]bool{}
	var photoBonuses []preparedPhotoBonus
	accessoryCounts := map[string]int64{}
	originalIDs := make([]string, len(positions))
	for i, p := range positions {
		originalIDs[i] = byCard[p.GetCardId()].GetCharacterId()
	}
	appearances, err := questDeckCharacterAssets(ctx, positions, originalIDs, characters, byMasterCharacter)
	if err != nil {
		return nil, err
	}
	extra, err := prepareExtraLiveAppearances(ctx, suid, music, characters, byMasterCharacter, appearances)
	if err != nil {
		return nil, err
	}
	self.UserDeck.ExtraCharacterInfos = extra
	for i, p := range positions {
		if p.GetPosition() != int32(i+1) || seenCards[p.GetCardId()] {
			return nil, viber.InvalidArgumentf("live positions 1..5 and cards must be unique")
		}
		seenCards[p.GetCardId()] = true
		u, m := ownedByID[p.GetCardId()], byCard[p.GetCardId()]
		if u == nil || m == nil {
			return nil, viber.FailedPreconditionf("live card %q is not owned or not in master", p.GetCardId())
		}
		uc, mc := byCharacter[m.GetCharacterId()], byMasterCharacter[m.GetCharacterId()]
		if uc == nil || mc == nil {
			return nil, viber.FailedPreconditionf("live character is missing")
		}
		c := &papi.LiveDeckCard{Index: int32(i + 1), CardId: u.GetCardId(), Level: u.GetLevel(), Rarity: u.GetRarity(), EvolutionLevel: u.GetEvolutionLevel(), Vocal: u.GetVocal(), Dance: u.GetDance(), Visual: u.GetVisual(), Stamina: u.GetStamina(), Mental: int64(setting.GetCardDefaultMental()), Technique: int64(setting.GetCardDefaultTechnique())}
		if c.GetLevel() <= 0 {
			return nil, viber.FailedPreconditionf("live card has no effective level")
		}
		bonuses := map[penum.PhotoAbilityType]int64{}
		for k, v := range global {
			bonuses[k] = v
		}
		for typ, v := range staffBonus {
			flat, _, ok := parameterPhotoTypes(typ)
			if !ok {
				return nil, viber.FailedPreconditionf("unsupported staff parameter %v", typ)
			}
			if err := addLiveBonus(bonuses, flat, v); err != nil {
				return nil, err
			}
		}
		for _, level := range companyLevels {
			if level.GetRequiredPoint() <= uc.GetCompanyEnjoyPoint() && level.GetBonusParameterType() != penum.ParameterType_ParameterType_Unknown {
				_, ratio, ok := parameterPhotoTypes(level.GetBonusParameterType())
				if !ok {
					return nil, viber.FailedPreconditionf("unsupported company parameter %v", level.GetBonusParameterType())
				}
				if err := addLiveBonus(bonuses, ratio, int64(level.GetBonusParameterUpPermil())); err != nil {
					return nil, err
				}
			}
		}
		if err := prepareLiveEquipment(ctx, suid, q, p, u, mc, m.GetType(), c, bonuses, equipmentBoost, &photoBonuses, seenPhotos, accessoryCounts); err != nil {
			return nil, err
		}
		levels := []int32{u.GetSkillLevel1(), u.GetSkillLevel2(), u.GetSkillLevel3(), u.GetSkillLevel4()}
		ids := []string{u.GetSkillId1(), u.GetSkillId2(), u.GetSkillId3(), u.GetSkillId4()}
		if err := prepareNativeSkills(ctx, c, m, ids, levels); err != nil {
			return nil, err
		}
		for j, ref := range globalPassives {
			c.Skills = append(c.Skills, &papi.LiveDeckCardSkill{Index: int32(2001 + j), SkillId: ref.GetSkillId(), SkillLevel: ref.GetSkillLevel(), PossessionType: penum.SkillPossessionType_SkillPossessionType_Ability, LiveAbilityId: ref.GetLiveAbilityId(), LiveAbilityLevel: ref.GetLiveAbilityLevel()})
		}
		appearance := appearances[i]
		// Prefetch is a projection; Start additionally validates ownership of the
		// exact chosen outfit/hair, including a substituted idol's live settings.
		if _, err := opuser.FindUserCostume(ctx, suid, appearance.GetCostumeId()); err != nil {
			return nil, err
		}
		if _, err := opuser.FindUserHair(ctx, suid, appearance.GetHairId()); err != nil {
			return nil, err
		}
		c.DisplayCharacterId, c.DisplayCostumeId, c.DisplayHairId = appearance.GetCharacterId(), appearance.GetCostumeId(), appearance.GetHairId()
		input.Cards[c.Index] = live.CardData{CharacterID: mc.GetId(), GroupID: mc.GetCharacterGroupId(), Type: m.GetType(), Attribute: liveCardAttribute(m), LightFans: int64(uc.GetLightFanAmount()), MiddleFans: int64(uc.GetMiddleFanAmount()), HeavyFans: int64(uc.GetHeavyFanAmount()), Bonuses: bonuses}
		self.UserDeck.Cards = append(self.UserDeck.Cards, c)
	}
	if err := applyPreparedPhotoBonuses(ctx, input, self, photoBonuses); err != nil {
		return nil, err
	}
	var characterAdvantage *pmaster.QuestCharacterAdvantage
	if q.GetQuestCharacterAdvantageId() != "" {
		characterAdvantage, err = opmaster.FindLiveCharacterAdvantage(ctx, q.GetQuestCharacterAdvantageId())
		if err != nil {
			return nil, err
		}
	}
	for _, c := range self.GetUserDeck().GetCards() {
		if err := applyPreparedStats(c, input.Cards[c.Index].Bonuses); err != nil {
			return nil, err
		}
		c.StartStamina = c.Stamina
	}
	for id, count := range accessoryCounts {
		owned, err := opuser.FindUserAccessory(ctx, suid, id)
		if err != nil {
			return nil, err
		}
		if owned.GetAmount() < count {
			return nil, viber.FailedPreconditionf("insufficient accessory copies")
		}
	}
	if npc != nil {
		if err := prepareNPC(ctx, input, npc, byCard, byMasterCharacter); err != nil {
			return nil, err
		}
	}
	for index, data := range input.Cards {
		data.CharacterAdvantagePermil, err = live.ResolveCharacterAdvantage(data, characterAdvantage)
		if err != nil {
			return nil, viber.FailedPreconditionf("%v", err)
		}
		input.Cards[index] = data
	}
	if err := prepareQuestBonus(ctx, input); err != nil {
		return nil, err
	}
	if err := resolveLiveSkills(ctx, input); err != nil {
		return nil, err
	}
	var actors []live.CardData
	var cards []*papi.LiveDeckCard
	for _, side := range input.Users {
		for _, c := range side.GetUserDeck().GetCards() {
			actors = append(actors, input.Cards[c.Index])
			cards = append(cards, c)
		}
	}
	audiences, err := live.AllocateAudience(actors, q.GetMaxCapacity(), setting)
	if err != nil {
		return nil, viber.FailedPreconditionf("%v", err)
	}
	for i, c := range cards {
		c.AudienceAmount = audiences[i]
	}
	return input, nil
}

// Attribute is stable card metadata, not the currently largest buffed stat.
// The master has no attribute field; its unique highest ratio identifies it.
// No current master ties exist; future ties prefer Vocal, Dance, then Visual.
func liveCardAttribute(m *pmaster.Card) penum.AttributeType {
	attribute, highest := penum.AttributeType_AttributeType_Vocal, m.GetVocalRatioPermil()
	if m.GetDanceRatioPermil() > highest {
		attribute, highest = penum.AttributeType_AttributeType_Dance, m.GetDanceRatioPermil()
	}
	if m.GetVisualRatioPermil() > highest {
		attribute = penum.AttributeType_AttributeType_Visual
	}
	return attribute
}

func liveStringKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func parameterPhotoTypes(typ penum.ParameterType) (penum.PhotoAbilityType, penum.PhotoAbilityType, bool) {
	switch typ {
	case penum.ParameterType_ParameterType_Dance:
		return penum.PhotoAbilityType_PhotoAbilityType_DanceAdd, penum.PhotoAbilityType_PhotoAbilityType_DanceMultiply, true
	case penum.ParameterType_ParameterType_Vocal:
		return penum.PhotoAbilityType_PhotoAbilityType_VocalAdd, penum.PhotoAbilityType_PhotoAbilityType_VocalMultiply, true
	case penum.ParameterType_ParameterType_Visual:
		return penum.PhotoAbilityType_PhotoAbilityType_VisualAdd, penum.PhotoAbilityType_PhotoAbilityType_VisualMultiply, true
	case penum.ParameterType_ParameterType_Stamina:
		return penum.PhotoAbilityType_PhotoAbilityType_StaminaAdd, penum.PhotoAbilityType_PhotoAbilityType_StaminaMultiply, true
	case penum.ParameterType_ParameterType_Mental:
		return penum.PhotoAbilityType_PhotoAbilityType_MentalAdd, penum.PhotoAbilityType_PhotoAbilityType_MentalMultiply, true
	case penum.ParameterType_ParameterType_Technique:
		return penum.PhotoAbilityType_PhotoAbilityType_TechniqueAdd, penum.PhotoAbilityType_PhotoAbilityType_TechniqueMultiply, true
	default:
		return penum.PhotoAbilityType_PhotoAbilityType_Unknown, penum.PhotoAbilityType_PhotoAbilityType_Unknown, false
	}
}

func accessoryPhotoTypes(typ penum.AccessoryParameterType) (penum.PhotoAbilityType, penum.PhotoAbilityType, bool) {
	switch typ {
	case penum.AccessoryParameterType_AccessoryParameterType_Dance:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Dance)
	case penum.AccessoryParameterType_AccessoryParameterType_Vocal:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Vocal)
	case penum.AccessoryParameterType_AccessoryParameterType_Visual:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Visual)
	case penum.AccessoryParameterType_AccessoryParameterType_Stamina:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Stamina)
	case penum.AccessoryParameterType_AccessoryParameterType_Mental:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Mental)
	case penum.AccessoryParameterType_AccessoryParameterType_Technique:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Technique)
	default:
		return penum.PhotoAbilityType_PhotoAbilityType_Unknown, penum.PhotoAbilityType_PhotoAbilityType_Unknown, false
	}
}

// The two enums are independent wire contracts; do not convert their numbers.
func liveAbilityPhotoType(typ penum.LiveAbilityType) (penum.PhotoAbilityType, bool) {
	switch typ {
	case penum.LiveAbilityType_LiveAbilityType_DanceAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_DanceAdd, true
	case penum.LiveAbilityType_LiveAbilityType_DanceMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_DanceMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_VocalAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_VocalAdd, true
	case penum.LiveAbilityType_LiveAbilityType_VocalMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_VocalMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_VisualAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_VisualAdd, true
	case penum.LiveAbilityType_LiveAbilityType_VisualMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_VisualMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_StaminaAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_StaminaAdd, true
	case penum.LiveAbilityType_LiveAbilityType_StaminaMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_StaminaMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_MentalAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_MentalAdd, true
	case penum.LiveAbilityType_LiveAbilityType_MentalMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_MentalMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_TechniqueAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_TechniqueAdd, true
	case penum.LiveAbilityType_LiveAbilityType_TechniqueMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_TechniqueMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_ManagerExp:
		return penum.PhotoAbilityType_PhotoAbilityType_ManagerExp, true
	case penum.LiveAbilityType_LiveAbilityType_Gold:
		return penum.PhotoAbilityType_PhotoAbilityType_Gold, true
	case penum.LiveAbilityType_LiveAbilityType_CardExp:
		return penum.PhotoAbilityType_PhotoAbilityType_CardExp, true
	case penum.LiveAbilityType_LiveAbilityType_BeatScoreAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_BeatScoreAdd, true
	case penum.LiveAbilityType_LiveAbilityType_BeatScoreMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_BeatScoreMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_ActiveSkillScoreAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreAdd, true
	case penum.LiveAbilityType_LiveAbilityType_ActiveSkillScoreMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_SpecialSkillScoreAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreAdd, true
	case penum.LiveAbilityType_LiveAbilityType_SpecialSkillScoreMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_CriticalScoreMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_CriticalScoreMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_AudienceAmountUp:
		return penum.PhotoAbilityType_PhotoAbilityType_AudienceAmountUp, true
	default:
		return penum.PhotoAbilityType_PhotoAbilityType_Unknown, false
	}
}

func addLiveBonus(b map[penum.PhotoAbilityType]int64, k penum.PhotoAbilityType, v int64) error {
	if v < 0 || b[k] > math.MaxInt64-v {
		return viber.FailedPreconditionf("negative or overflowing live bonus")
	}
	b[k] += v
	return nil
}

func applyPreparedStats(c *papi.LiveDeckCard, b map[penum.PhotoAbilityType]int64) error {
	values := []struct {
		value     *int64
		parameter penum.ParameterType
	}{
		{&c.Dance, penum.ParameterType_ParameterType_Dance}, {&c.Vocal, penum.ParameterType_ParameterType_Vocal},
		{&c.Visual, penum.ParameterType_ParameterType_Visual}, {&c.Stamina, penum.ParameterType_ParameterType_Stamina},
		{&c.Mental, penum.ParameterType_ParameterType_Mental}, {&c.Technique, penum.ParameterType_ParameterType_Technique},
	}
	for _, entry := range values {
		value := entry.value
		if *value < 0 {
			return viber.FailedPreconditionf("negative card base parameter")
		}
		flatType, ratioType, _ := parameterPhotoTypes(entry.parameter)
		flat, ratio := b[flatType], b[ratioType]
		n := new(big.Int).Add(big.NewInt(1000), big.NewInt(ratio))
		n.Mul(n, big.NewInt(*value))
		n.Quo(n, big.NewInt(1000))
		n.Add(n, big.NewInt(flat))
		if !n.IsInt64() || n.Sign() < 0 {
			return viber.FailedPreconditionf("live parameter overflows")
		}
		*value = n.Int64()
	}
	return nil
}

type liveEquipmentBoost struct{ accessoryPermil, photoLevel int64 }

func prepareOwnedLiveAbilities(ctx context.Context, owned []*ptransaction.UserCard, q *pmaster.Quest) (map[penum.PhotoAbilityType]int64, []*papi.LiveDeckCardSkill, liveEquipmentBoost, error) {
	var equipment liveEquipmentBoost
	ids := map[string]bool{}
	for _, c := range owned {
		if c.GetLiveAbilityId() != "" && c.GetLiveAbilityLevel() > 0 {
			ids[c.GetLiveAbilityId()] = true
		}
	}
	masters, err := opmaster.FindLiveAbilities(ctx, liveStringKeys(ids))
	if err != nil {
		return nil, nil, equipment, err
	}
	byID := map[string]*pmaster.LiveAbility{}
	for _, m := range masters {
		byID[m.GetId()] = m
	}
	bonuses := map[penum.PhotoAbilityType]int64{}
	var passives []*papi.LiveDeckCardSkill
	for _, c := range owned {
		if c.GetLiveAbilityId() == "" || c.GetLiveAbilityLevel() <= 0 {
			continue
		}
		var chosen *pmaster.LiveAbilityLevel
		for _, level := range byID[c.GetLiveAbilityId()].GetLevels() {
			if level.GetLevel() == c.GetLiveAbilityLevel() && level.GetRequiredCardLevel() <= c.GetLevel() {
				chosen = level
				break
			}
		}
		if chosen == nil {
			return nil, nil, equipment, viber.FailedPreconditionf("owned live ability level is invalid")
		}
		if chosen.GetMoodType() != penum.MoodType_MoodType_Unknown && chosen.GetMoodType() != q.GetMoodType() {
			continue
		}
		if chosen.GetType() == penum.LiveAbilityType_LiveAbilityType_PassiveSkill {
			passives = append(passives, &papi.LiveDeckCardSkill{SkillId: chosen.GetSkillId(), SkillLevel: chosen.GetValue(), LiveAbilityId: c.GetLiveAbilityId(), LiveAbilityLevel: chosen.GetLevel()})
			continue
		}
		var target *int64
		switch chosen.GetType() {
		case penum.LiveAbilityType_LiveAbilityType_AccessoryParameterUp:
			target = &equipment.accessoryPermil
		case penum.LiveAbilityType_LiveAbilityType_PhotoLevelUp:
			target = &equipment.photoLevel
		}
		if target != nil {
			if chosen.GetValue() < 0 || *target > math.MaxInt64-int64(chosen.GetValue()) {
				return nil, nil, equipment, viber.FailedPreconditionf("equipment bonus overflows")
			}
			*target += int64(chosen.GetValue())
			continue
		}
		photoType, ok := liveAbilityPhotoType(chosen.GetType())
		if !ok {
			return nil, nil, equipment, status.Errorf(codes.Unimplemented, "live ability %v is unsupported", chosen.GetType())
		}
		if err := addLiveBonus(bonuses, photoType, int64(chosen.GetValue())); err != nil {
			return nil, nil, equipment, err
		}
	}
	return bonuses, passives, equipment, nil
}

func prepareLiveEquipment(ctx context.Context, suid string, q *pmaster.Quest, p *ptransaction.UserDeckPosition, u *ptransaction.UserCard, character *pmaster.Character, cardType penum.CardType, c *papi.LiveDeckCard, b map[penum.PhotoAbilityType]int64, equipment liveEquipmentBoost, photos *[]preparedPhotoBonus, seenPhotos map[string]bool, counts map[string]int64) error {
	for part, id := range []string{p.GetPart1AccessoryId(), p.GetPart2AccessoryId()} {
		if id == "" {
			continue
		}
		a, err := opmaster.FindAccessory(ctx, id)
		if err != nil {
			return err
		}
		category := []penum.AccessoryCategoryType{penum.AccessoryCategoryType_AccessoryCategoryType_Part1, penum.AccessoryCategoryType_AccessoryCategoryType_Part2}[part]
		if a.GetCategoryType() != category || a.GetCharacterId() != "" && a.GetCharacterId() != character.GetId() || a.GetCharacterGroupId() != "" && a.GetCharacterGroupId() != character.GetCharacterGroupId() {
			return viber.FailedPreconditionf("accessory cannot be equipped by this card")
		}
		counts[id]++
		for _, pair := range []struct {
			typ         penum.AccessoryParameterType
			flat, ratio int32
		}{{a.GetParam1Type(), a.GetParam1Value(), a.GetParam1Permil()}, {a.GetParam2Type(), a.GetParam2Value(), a.GetParam2Permil()}} {
			if pair.typ == penum.AccessoryParameterType_AccessoryParameterType_Unknown {
				continue
			}
			flat, ratio := int64(pair.flat), int64(pair.ratio)
			boost := equipment.accessoryPermil
			if boost > 0 {
				if boost > math.MaxInt64-1000 || flat > math.MaxInt64/(1000+boost) || ratio > math.MaxInt64/(1000+boost) {
					return viber.FailedPreconditionf("accessory bonus overflows")
				}
				flat = flat * (1000 + boost) / 1000
				ratio = ratio * (1000 + boost) / 1000
			}
			flatType, ratioType, ok := accessoryPhotoTypes(pair.typ)
			if !ok {
				return viber.FailedPreconditionf("unsupported accessory parameter %v", pair.typ)
			}
			if err := addLiveBonus(b, flatType, flat); err != nil {
				return err
			}
			if err := addLiveBonus(b, ratioType, ratio); err != nil {
				return err
			}
		}
	}
	if len(p.GetPhotoIds()) > int(u.GetPhotoEquipableCount()) {
		return viber.FailedPreconditionf("insufficient photo slots")
	}
	var retouched int
	for _, id := range p.GetPhotoIds() {
		if id == "" || seenPhotos[id] {
			return viber.FailedPreconditionf("photo must not be equipped twice")
		}
		seenPhotos[id] = true
		photo, err := opuser.FindUserPhoto(ctx, suid, id)
		if err != nil {
			return err
		}
		if len(photo.GetCharacterIds()) > 0 && !slices.Contains(photo.GetCharacterIds(), character.GetId()) {
			return viber.FailedPreconditionf("photo is not equipable by this idol")
		}
		if photo.GetPhotoRecipeId() != "" {
			retouched++
		}
		if retouched > 1 {
			return viber.FailedPreconditionf("only one retouched photo may be equipped")
		}
		ids := map[string]bool{}
		for _, ability := range photo.GetAbilities() {
			if ability.GetIsAvailable() {
				ids[ability.GetPhotoAbilityId()] = true
			}
		}
		definitions, err := opmaster.FindPhotoAbilities(ctx, liveStringKeys(ids))
		if err != nil {
			return err
		}
		byID := map[string]*pmaster.PhotoAbility{}
		for _, d := range definitions {
			byID[d.GetId()] = d
		}
		for _, ability := range photo.GetAbilities() {
			if !ability.GetIsAvailable() {
				continue
			}
			d := byID[ability.GetPhotoAbilityId()]
			if d == nil {
				return viber.FailedPreconditionf("photo ability master is missing")
			}
			if d.GetMoodType() != penum.MoodType_MoodType_Unknown && d.GetMoodType() != q.GetMoodType() {
				continue
			}
			if d.GetConditionCardType() != penum.CardType_CardType_Unknown {
				if cardType != d.GetConditionCardType() {
					continue
				}
			}
			kind := penum.LiveType_LiveType_Score
			if q.GetClearScore() == 0 {
				kind = penum.LiveType_LiveType_Battle
			}
			if d.GetConditionLiveType() != penum.LiveType_LiveType_Unknown && d.GetConditionLiveType() != kind {
				continue
			}
			level := int64(photo.GetLevel()) + equipment.photoLevel
			if level <= 0 || level > math.MaxInt32 {
				return viber.FailedPreconditionf("invalid effective photo level")
			}
			effect, err := viber.PhotoAbilityEffectValue(d, int32(level), ability.GetGrade())
			if err != nil {
				return err
			}
			*photos = append(*photos, preparedPhotoBonus{source: c.GetIndex(), definition: d, amount: effect})
		}
	}
	return nil
}

func preparedPhotoBonusType(typ penum.PhotoAbilityType) bool {
	switch typ {
	case penum.PhotoAbilityType_PhotoAbilityType_DanceAdd,
		penum.PhotoAbilityType_PhotoAbilityType_DanceMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_VocalAdd,
		penum.PhotoAbilityType_PhotoAbilityType_VocalMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_VisualAdd,
		penum.PhotoAbilityType_PhotoAbilityType_VisualMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_StaminaAdd,
		penum.PhotoAbilityType_PhotoAbilityType_StaminaMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_MentalAdd,
		penum.PhotoAbilityType_PhotoAbilityType_MentalMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_TechniqueAdd,
		penum.PhotoAbilityType_PhotoAbilityType_TechniqueMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_ManagerExp,
		penum.PhotoAbilityType_PhotoAbilityType_Gold,
		penum.PhotoAbilityType_PhotoAbilityType_CardExp,
		penum.PhotoAbilityType_PhotoAbilityType_BeatScoreAdd,
		penum.PhotoAbilityType_PhotoAbilityType_BeatScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreAdd,
		penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreAdd,
		penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_CriticalScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_AudienceAmountUp,
		penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill1,
		penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill2,
		penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill3:
		return true
	default:
		return false
	}
}

type preparedPhotoBonus struct {
	source     int32
	definition *pmaster.PhotoAbility
	amount     int64
}

// Resolve all recipients before finalizing any card's stats, so a neighbor's
// photo affects earlier and later positions identically. Definitions are loaded
// once per preparation, never from inside the per-beat engine.
func applyPreparedPhotoBonuses(ctx context.Context, input *live.Input, user *papi.LiveUserInfo, bonuses []preparedPhotoBonus) error {
	ids := map[string]bool{}
	for _, bonus := range bonuses {
		if bonus.definition.GetAbilityTargetId() != "" {
			ids[bonus.definition.GetAbilityTargetId()] = true
		}
	}
	targets := map[string]*pmaster.PhotoAbilityTarget{}
	if len(ids) > 0 {
		rows, err := opmaster.FindLivePhotoTargets(ctx, liveStringKeys(ids))
		if err != nil {
			return err
		}
		for _, row := range rows {
			targets[row.GetId()] = row
		}
		if len(targets) != len(ids) {
			return viber.FailedPreconditionf("photo target master is missing")
		}
	}
	cards := user.GetUserDeck().GetCards()
	byIndex := make(map[int32]*papi.LiveDeckCard, len(cards))
	for _, card := range cards {
		byIndex[card.GetIndex()] = card
	}
	for _, bonus := range bonuses {
		d := bonus.definition
		if bonus.amount < 0 {
			return viber.FailedPreconditionf("negative photo effect")
		}
		recipients, err := live.PhotoTargetIndexes(cards, input.Cards, bonus.source, targets[d.GetAbilityTargetId()])
		if err != nil {
			return viber.FailedPreconditionf("%v", err)
		}
		for _, index := range recipients {
			data := input.Cards[index]
			switch d.GetAbilityType() {
			case penum.PhotoAbilityType_PhotoAbilityType_PassiveSkill:
				if bonus.amount <= 0 || bonus.amount > math.MaxInt32 {
					return viber.FailedPreconditionf("invalid photo passive level")
				}
				card := byIndex[index]
				photoIndex := int32(1001)
				for _, skill := range card.GetSkills() {
					if skill.GetPossessionType() == penum.SkillPossessionType_SkillPossessionType_Photo {
						photoIndex++
					}
				}
				card.Skills = append(card.Skills, &papi.LiveDeckCardSkill{Index: photoIndex, SkillId: d.GetSkillId(), SkillLevel: int32(bonus.amount), PossessionType: penum.SkillPossessionType_SkillPossessionType_Photo})
			case penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectValueIncrease,
				penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectCountIncrease,
				penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectValueIncrease,
				penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectCountIncrease:
				if bonus.amount > math.MaxInt32 || d.GetStatusEffectType() == penum.StatusEffectType_StatusEffectType_Unknown {
					return viber.FailedPreconditionf("invalid status-restricted photo modifier")
				}
				data.StrengthModifiers = append(data.StrengthModifiers, live.StrengthModifier{Type: d.GetAbilityType(), StatusTypes: []penum.StatusEffectType{d.GetStatusEffectType()}, Amount: int32(bonus.amount)})
			default:
				if !preparedPhotoBonusType(d.GetAbilityType()) {
					return status.Errorf(codes.Unimplemented, "photo ability %v is unsupported", d.GetAbilityType())
				}
				if err := addLiveBonus(data.Bonuses, d.GetAbilityType(), bonus.amount); err != nil {
					return err
				}
			}
			input.Cards[index] = data
		}
	}
	return nil
}

func prepareNativeSkills(ctx context.Context, c *papi.LiveDeckCard, m *pmaster.Card, ids []string, levels []int32) error {
	_, unlockedIDs, _, err := usercard.CalcUnlocksWithEvolution(ctx, c.GetLevel(), c.GetEvolutionLevel(), m)
	if err != nil {
		return err
	}
	native := []*papi.LiveDeckCardSkill{}
	masterIDs := []string{m.GetSkillId1(), m.GetSkillId2(), m.GetSkillId3(), m.GetSkillId4()}
	for i, id := range ids {
		if id == "" {
			id = masterIDs[i]
		}
		if unlockedIDs[i] == "" || id == "" {
			continue
		}
		level := levels[i]
		if level == 0 {
			return viber.FailedPreconditionf("learned native skill has an invalid level")
		}
		native = append(native, &papi.LiveDeckCardSkill{Index: int32(i + 1), SkillId: id, SkillLevel: level, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card})
	}
	c.Skills = append(native, c.Skills...)
	return nil
}

// NPC visual defaults are independent of the requesting user's preferences.
func prepareNPCAppearance(ctx context.Context, c *papi.LiveDeckCard, m *pmaster.Character) error {
	costumeID := m.GetDefaultCostumeId()
	costume, err := opmaster.FindCostume(ctx, costumeID)
	if err != nil {
		return err
	}
	if costume.GetCharacterId() != m.GetId() || strings.HasPrefix(costume.GetCostumeTypeId(), "sd-") {
		return viber.FailedPreconditionf("invalid live costume")
	}
	hairID := costume.GetDefaultHairId()
	hair, err := opmaster.FindHair(ctx, hairID)
	if err != nil {
		return err
	}
	if hair.GetCharacterId() != m.GetId() || slices.Contains(hair.GetNotWearableCostumeIds(), costumeID) || len(hair.GetWearableCostumeIds()) > 0 && !slices.Contains(hair.GetWearableCostumeIds(), costumeID) {
		return viber.FailedPreconditionf("invalid live hair")
	}
	c.DisplayCharacterId, c.DisplayCostumeId, c.DisplayHairId = m.GetId(), costumeID, hairID
	return nil
}

func prepareNPC(ctx context.Context, input *live.Input, roster *papi.QuestOpponentInfo, masters map[string]*pmaster.Card, characters map[string]*pmaster.Character) error {
	bonus, err := live.NPCBalance(input.Quest, input.Setting)
	if err != nil {
		return err
	}
	side := &papi.LiveUserInfo{Index: 2, IsNpc: true, Name: roster.GetName(), UserDeck: &papi.LiveUserDeck{}}
	sort.Slice(roster.CardInfos, func(i, j int) bool { return roster.CardInfos[i].GetPosition() < roster.CardInfos[j].GetPosition() })
	for _, r := range roster.GetCardInfos() {
		m := masters[r.GetCardId()]
		if m == nil {
			return viber.FailedPreconditionf("NPC card master missing")
		}
		if r.GetEvolutionLevel() != 0 {
			return status.Error(codes.Unimplemented, "NPC evolution preparation is not supported")
		}
		base, err := usercard.CalcUserCardBaseAttr(ctx, m, r.GetLevel(), r.GetRarity())
		if err != nil {
			return err
		}
		for _, v := range []int64{base.Vocal, base.Dance, base.Visual} {
			if v > math.MaxInt64-bonus.Primary {
				return viber.FailedPreconditionf("NPC stat overflows")
			}
		}
		if base.Stamina > math.MaxInt64-bonus.Stamina {
			return viber.FailedPreconditionf("NPC stamina overflows")
		}
		c := &papi.LiveDeckCard{Index: 5 + r.GetPosition(), CardId: r.GetCardId(), Level: r.GetLevel(), Rarity: r.GetRarity(), Vocal: base.Vocal + bonus.Primary, Dance: base.Dance + bonus.Primary, Visual: base.Visual + bonus.Primary, Stamina: base.Stamina + bonus.Stamina, Mental: bonus.Mental, Technique: bonus.Technique}
		c.StartStamina = c.Stamina
		if err := prepareNativeSkills(ctx, c, m, []string{m.GetSkillId1(), m.GetSkillId2(), m.GetSkillId3(), m.GetSkillId4()}, []int32{-1, -1, -1, -1}); err != nil {
			return err
		}
		character := characters[m.GetCharacterId()]
		if character == nil {
			return viber.FailedPreconditionf("NPC character missing")
		}
		if err := prepareNPCAppearance(ctx, c, character); err != nil {
			return err
		}
		input.Cards[c.Index] = live.CardData{CharacterID: m.GetCharacterId(), GroupID: character.GetCharacterGroupId(), Type: m.GetType(), Attribute: liveCardAttribute(m), LightFans: int64(r.GetLightFanAmount()), MiddleFans: int64(r.GetMiddleFanAmount()), HeavyFans: int64(r.GetHeavyFanAmount()), Bonuses: map[penum.PhotoAbilityType]int64{}}
		side.UserDeck.Cards = append(side.UserDeck.Cards, c)
	}
	input.Users = append(input.Users, side)
	return nil
}

func prepareQuestBonus(ctx context.Context, input *live.Input) error {
	if input.Quest.GetLiveBonusGroupId() == "" {
		return nil
	}
	group, err := opmaster.FindLiveBonusGroup(ctx, input.Quest.GetLiveBonusGroupId())
	if err != nil {
		return err
	}
	bonuses, err := opmaster.FindLiveBonuses(ctx, group.GetLiveBonusIds())
	if err != nil {
		return err
	}
	if len(bonuses) != len(group.GetLiveBonusIds()) {
		return viber.FailedPreconditionf("live bonus group is incomplete")
	}
	byID := map[string]*pmaster.LiveBonus{}
	ids := map[string]bool{}
	for _, b := range bonuses {
		byID[b.GetId()] = b
		ids[b.GetLiveAbilityId()] = true
	}
	abilities, err := opmaster.FindLiveAbilities(ctx, liveStringKeys(ids))
	if err != nil {
		return err
	}
	byAbility := map[string]*pmaster.LiveAbility{}
	for _, a := range abilities {
		byAbility[a.GetId()] = a
	}
	for i, id := range group.GetLiveBonusIds() {
		b := byID[id]
		var level *pmaster.LiveAbilityLevel
		for _, l := range byAbility[b.GetLiveAbilityId()].GetLevels() {
			if l.GetLevel() == b.GetLiveAbilityLevel() {
				level = l
				break
			}
		}
		if level == nil {
			return viber.FailedPreconditionf("quest live bonus level missing")
		}
		if level.GetType() != penum.LiveAbilityType_LiveAbilityType_PassiveSkill {
			return status.Error(codes.Unimplemented, "non-passive quest bonus is not yet supported")
		}
		for _, u := range input.Users {
			u.UserDeck.BonusSkills = append(u.UserDeck.BonusSkills, &papi.LiveDeckCardSkill{Index: 3001 + int32(i), SkillId: level.GetSkillId(), SkillLevel: level.GetValue(), PossessionType: penum.SkillPossessionType_SkillPossessionType_Quest, LiveAbilityId: b.GetLiveAbilityId(), LiveAbilityLevel: b.GetLiveAbilityLevel()})
		}
	}
	return nil
}

func resolveLiveSkills(ctx context.Context, input *live.Input) error {
	ids := map[string]bool{}
	for _, u := range input.Users {
		for _, c := range u.GetUserDeck().GetCards() {
			for _, s := range c.GetSkills() {
				ids[s.GetSkillId()] = true
			}
		}
		for _, s := range u.GetUserDeck().GetBonusSkills() {
			ids[s.GetSkillId()] = true
		}
	}
	skills, err := opmaster.FindLiveSkills(ctx, liveStringKeys(ids))
	if err != nil {
		return err
	}
	for _, s := range skills {
		input.Skills[s.GetId()] = s
	}
	if len(skills) != len(ids) {
		return viber.FailedPreconditionf("live references missing skill definitions")
	}
	efficacyIDs, triggerIDs, targetIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	collect := func(ref *papi.LiveDeckCardSkill, cardLevel int32) error {
		definition := input.Skills[ref.GetSkillId()]
		if ref.GetSkillLevel() < 0 {
			ref.SkillLevel = 0
			for _, l := range definition.GetLevels() {
				if l.GetRequiredCardLevel() <= cardLevel {
					ref.SkillLevel = max(ref.SkillLevel, l.GetLevel())
				}
			}
		}
		var level *pmaster.SkillLevel
		for _, l := range definition.GetLevels() {
			if l.GetLevel() == ref.GetSkillLevel() {
				level = l
				break
			}
		}
		if level == nil {
			return viber.FailedPreconditionf("invalid skill level for %q", ref.GetSkillId())
		}
		if ref.GetPossessionType() == penum.SkillPossessionType_SkillPossessionType_Card && level.GetRequiredCardLevel() > cardLevel {
			return viber.FailedPreconditionf("native skill level exceeds the card's effective level")
		}
		triggerIDs[level.GetTriggerId()] = true
		for _, d := range level.GetSkillDetails() {
			efficacyIDs[d.GetEfficacyId()] = true
			triggerIDs[d.GetTriggerId()] = true
		}
		for _, id := range ref.GetRewrittenEfficacyIds() {
			efficacyIDs[id] = true
		}
		return nil
	}
	for _, u := range input.Users {
		for _, c := range u.GetUserDeck().GetCards() {
			for _, s := range c.GetSkills() {
				if err := collect(s, c.GetLevel()); err != nil {
					return err
				}
				switch s.GetIndex() {
				case 1:
					c.SkillLevel1 = s.GetSkillLevel()
				case 2:
					c.SkillLevel2 = s.GetSkillLevel()
				case 3:
					c.SkillLevel3 = s.GetSkillLevel()
				case 4:
					c.SkillLevel4 = s.GetSkillLevel()
				}
			}
		}
		for _, s := range u.GetUserDeck().GetBonusSkills() {
			if err := collect(s, math.MaxInt32); err != nil {
				return err
			}
		}
	}
	efficacies, err := opmaster.FindLiveEfficacies(ctx, liveStringKeys(efficacyIDs))
	if err != nil {
		return err
	}
	for _, e := range efficacies {
		input.Efficacies[e.GetId()] = e
		targetIDs[e.GetSkillTargetId()] = true
		// Conditional score encodes an additional trigger inside the efficacy ID
		// rather than SkillLevel/SkillDetail. Resolve it before entering the engine.
		if strings.HasPrefix(e.GetId(), "ef-score_get_by_trigger-") {
			parts := strings.Split(strings.TrimPrefix(e.GetId(), "ef-"), "-")
			if len(parts) >= 6 {
				triggerIDs[strings.Join(parts[2:len(parts)-2], "-")] = true
			}
		}
	}
	if len(efficacies) != len(liveStringKeys(efficacyIDs)) {
		return viber.FailedPreconditionf("live efficacy definition is missing")
	}
	for _, efficacy := range efficacies {
		destination, conversion, err := live.ConversionStatusEfficacyType(efficacy)
		if err != nil {
			return viber.FailedPreconditionf("%v", err)
		}
		if !conversion {
			continue
		}
		found := false
		for _, candidate := range input.Efficacies {
			if candidate.GetType() == destination {
				found = true
				break
			}
		}
		if found {
			continue
		}
		name := efficacy.GetId()[strings.LastIndex(efficacy.GetId(), "-")+1:]
		definition, err := opmaster.FindLiveStatusEfficacy(ctx, name)
		if err != nil {
			return err
		}
		if definition.GetType() != destination {
			return viber.FailedPreconditionf("conversion destination master mismatch")
		}
		input.Efficacies[definition.GetId()] = definition
	}
	triggers, err := opmaster.FindLiveTriggers(ctx, liveStringKeys(triggerIDs))
	if err != nil {
		return err
	}
	for _, t := range triggers {
		input.Triggers[t.GetId()] = t
	}
	if len(triggers) != len(liveStringKeys(triggerIDs)) {
		return viber.FailedPreconditionf("live trigger definition is missing")
	}
	targets, err := opmaster.FindLiveTargets(ctx, liveStringKeys(targetIDs))
	if err != nil {
		return err
	}
	for _, t := range targets {
		input.Targets[t.GetId()] = t
	}
	if len(targets) != len(liveStringKeys(targetIDs)) {
		return viber.FailedPreconditionf("live target definition is missing")
	}
	return nil
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
