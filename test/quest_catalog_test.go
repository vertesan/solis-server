package test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

func TestQuestBossPreviewScenario(t *testing.T) {
	setupTestUser(t)
	areas, quests := questCatalogFixture(t)
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	var selected *pmaster.Quest
	var selectedArea *pmaster.Area
	var expected *papi.QuestOpponentInfo
	for _, area := range main {
		for _, id := range area.GetQuestIds() {
			q := quests[id]
			if q.GetClearScore() != 0 {
				continue
			}
			roster, err := live.NPCRoster(id)
			if err == nil {
				selected, selectedArea, expected = q, area, roster
				break
			}
		}
		if selected != nil {
			break
		}
	}
	if selected == nil {
		t.Fatal("master catalog has no known main boss roster")
	}
	for _, area := range main {
		if area.GetOrder() < selectedArea.GetOrder() {
			questSeedFullArea(t, area)
		}
	}
	maximum, err := opmaster.FindMaxManagerLevel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	questSetManagerLevel(t, maximum.GetLevel())
	client := papi.NewQuestClient(testConfig.GrpcConn)
	response := profileCall(t, client.ListMain, &papi.Empty{})
	area := questFindMainArea(t, response.GetAreaGroups(), selectedArea.GetId())
	var found *papi.QuestInfo
	for _, q := range area.GetQuests() {
		if q.GetQuestId() == selected.GetId() {
			found = q
			break
		}
	}
	if found == nil || len(found.GetOpponentInfo().GetCardInfos()) != 5 || !proto.Equal(found.GetOpponentInfo(), expected) {
		t.Fatal("main boss preview omitted or changed its known NPC unit")
	}
	top := profileCall(t, client.Top, &papi.Empty{})
	area = questFindMainArea(t, top.GetMainAreaGroups(), selectedArea.GetId())
	for _, q := range area.GetQuests() {
		if q.GetQuestId() == selected.GetId() {
			if !proto.Equal(q.GetOpponentInfo(), expected) {
				t.Fatal("Top and ListMain disagree on NPC preview")
			}
			return
		}
	}
	t.Fatal("Top omitted the selected boss")
}

func TestQuestMainProgressionScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewQuestClient(testConfig.GrpcConn)
	areas, _ := questCatalogFixture(t)
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	if len(main) < 2 || len(main[0].GetQuestIds()) < 2 {
		t.Fatal("main master needs two areas and multiple first-area quests")
	}
	top := profileCall(t, client.Top, &papi.Empty{})
	listed := profileCall(t, client.ListMain, &papi.Empty{})
	if len(top.GetMainAreaGroups()) != len(listed.GetAreaGroups()) {
		t.Fatal("Top and ListMain disagree on visible groups")
	}
	for i, group := range top.GetMainAreaGroups() {
		if !proto.Equal(group, listed.GetAreaGroups()[i]) {
			t.Fatal("Top and ListMain disagree on main progression")
		}
	}
	assertQuestOfflineTop(t, top)
	first := questFindMainArea(t, listed.GetAreaGroups(), main[0].GetId())
	if !first.GetUnlocked() || len(first.GetQuests()) != len(main[0].GetQuestIds()) {
		t.Fatal("fresh first area is not completely listed and unlocked")
	}
	for _, quest := range first.GetQuests() {
		if quest.GetCleared() || !quest.GetIsPlayable() {
			t.Fatalf("fresh first area must allow each quest in any order: %s", quest.GetQuestId())
		}
	}
	if questFindMainArea(t, listed.GetAreaGroups(), main[1].GetId()).GetUnlocked() {
		t.Fatal("second main area unlocked before the first is cleared")
	}
	// Clear the last quest first: partial progression must not force an order
	// within this area or prematurely unlock the next area.
	lastID := main[0].GetQuestIds()[len(main[0].GetQuestIds())-1]
	questSeedCleared(t, lastID)
	questSeedArea(t, main[0], 1, 0, time.Now())
	partial := profileCall(t, client.ListMain, &papi.Empty{})
	first = questFindMainArea(t, partial.GetAreaGroups(), main[0].GetId())
	for _, quest := range first.GetQuests() {
		if quest.GetCleared() != (quest.GetQuestId() == lastID) || quest.GetIsPlayable() == quest.GetCleared() {
			t.Fatalf("partial main area lost independent quest progress: %s", quest.GetQuestId())
		}
		assertQuestNoScore(t, quest)
	}
	if questFindMainArea(t, partial.GetAreaGroups(), main[1].GetId()).GetUnlocked() {
		t.Fatal("partial first area unlocked the next main area")
	}
	questSeedFullArea(t, main[0])
	complete := profileCall(t, client.ListMain, &papi.Empty{})
	if !questFindMainArea(t, complete.GetAreaGroups(), main[1].GetId()).GetUnlocked() {
		t.Fatal("clearing every first-area quest did not unlock the second area")
	}

	// Locate the first manager-level gate directly in the master catalog.
	gateIndex, requiredLevel := -1, int32(0)
	for i, area := range main {
		if area.GetUnlockConditionId() == "" {
			continue
		}
		condition, err := opmaster.FindCondition(context.Background(), area.GetUnlockConditionId())
		if err != nil {
			t.Fatal(err)
		}
		for _, setting := range condition.GetSettings() {
			if level := setting.GetManagerLevel().GetLevelGte(); level > 1 {
				gateIndex, requiredLevel = i, level
				break
			}
		}
		if gateIndex >= 0 {
			break
		}
	}
	if gateIndex < 0 {
		t.Fatal("main catalog has no manager-level gate to verify")
	}
	for _, area := range main[:gateIndex] {
		questSeedFullArea(t, area)
	}
	questSetManagerLevel(t, requiredLevel-1)
	locked := profileCall(t, client.ListMain, &papi.Empty{})
	if questFindMainArea(t, locked.GetAreaGroups(), main[gateIndex].GetId()).GetUnlocked() {
		t.Fatal("completed previous areas bypassed the manager-level gate")
	}
	questSetManagerLevel(t, requiredLevel)
	unlocked := profileCall(t, client.ListMain, &papi.Empty{})
	if !questFindMainArea(t, unlocked.GetAreaGroups(), main[gateIndex].GetId()).GetUnlocked() {
		t.Fatal("meeting the master manager-level requirement did not unlock the main area")
	}
}

func TestQuestDailyPassScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	areas, quests := questCatalogFixture(t)
	daily := questPermanentDailyAreas(areas)
	if len(daily) != 3 {
		t.Fatalf("expected three permanent daily types, got %d", len(daily))
	}
	fresh := profileCall(t, client.ListDaily, &papi.Empty{})
	assertQuestDailyAreas(t, fresh.GetAreas(), daily)
	if len(fresh.GetBulkDailyInfos()) != 0 {
		t.Fatal("fresh user has bulk daily history without playing a live")
	}
	// Unlock the permanent daily feature via main progression, leaving each
	// daily stage uncleared so only its unconditional first stage is visible.
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	var requiredMainOrder int32
	for _, dailyArea := range daily {
		firstQuest := quests[dailyArea.GetQuestIds()[0]]
		condition, err := opmaster.FindCondition(ctx, firstQuest.GetUnlockConditionId())
		if err != nil {
			t.Fatal(err)
		}
		for _, setting := range condition.GetSettings() {
			for _, area := range main {
				if area.GetId() == setting.GetClearArea().GetAreaId() {
					requiredMainOrder = max(requiredMainOrder, area.GetOrder())
				}
			}
		}
	}
	if requiredMainOrder == 0 {
		t.Fatal("daily first-stage master conditions contain no main-area prerequisite")
	}
	for _, area := range main {
		if area.GetOrder() <= requiredMainOrder {
			questSeedFullArea(t, area)
		}
	}
	maximum, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	questSetManagerLevel(t, maximum.GetLevel())
	unlocked := profileCall(t, client.ListDaily, &papi.Empty{})
	for _, area := range unlocked.GetAreas() {
		if len(area.GetQuests()) != 1 || !area.GetQuests()[0].GetIsPlayable() || area.GetQuests()[0].GetPlayableCount() != 5 {
			t.Fatalf("uncleared daily must expose its first stage and five free plays: %s", area.GetAreaId())
		}
	}
	selected := daily[0]
	if len(selected.GetQuestIds()) < 2 {
		t.Fatal("daily catalog needs a second stage")
	}
	second := quests[selected.GetQuestIds()[1]]
	condition, err := opmaster.FindCondition(ctx, second.GetUnlockConditionId())
	if err != nil {
		t.Fatal(err)
	}
	var requiredLevel int32
	for _, setting := range condition.GetSettings() {
		requiredLevel = max(requiredLevel, setting.GetManagerLevel().GetLevelGte())
	}
	if requiredLevel <= 1 {
		t.Fatal("second permanent daily stage has no manager-level gate")
	}
	questSeedCleared(t, selected.GetQuestIds()[0])
	questSeedArea(t, selected, 1, 2, time.Now())
	questSetManagerLevel(t, requiredLevel-1)
	partial := profileCall(t, client.ListDaily, &papi.Empty{})
	area := questFindArea(t, partial.GetAreas(), selected.GetId())
	if len(area.GetQuests()) != 2 || area.GetQuests()[1].GetQuestId() != second.GetId() || area.GetQuests()[1].GetIsPlayable() {
		t.Fatal("daily stage view and unlock conditions were not evaluated separately")
	}
	if area.GetQuests()[0].GetPlayableCount() != 3 {
		t.Fatal("daily free-pass count did not subtract today's two plays")
	}
	questSetManagerLevel(t, requiredLevel)
	// Paid tickets must neither be counted as FREE passes nor consumed while
	// browsing. Resolve the ticket from the area's master item type.
	items, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ticketID string
	for _, item := range items {
		if item.GetType() == selected.GetDailyTicketItemType() {
			ticketID = item.GetId()
			break
		}
	}
	if ticketID == "" {
		t.Fatal("daily ticket type has no master item")
	}
	setCardTrainingItem(t, ticketID, 7)
	questSeedArea(t, selected, 1, 5, time.Now())
	exhausted := profileCall(t, client.ListDaily, &papi.Empty{})
	for _, quest := range questFindArea(t, exhausted.GetAreas(), selected.GetId()).GetQuests() {
		if !quest.GetIsPlayable() || quest.GetPlayableCount() != 0 {
			t.Fatal("exhausted FREE passes must not relock an eligible daily quest")
		}
	}
	questSetManagerLevel(t, maximum.GetLevel())
	for _, master := range daily {
		questSeedCleared(t, master.GetQuestIds()[0])
		questSeedArea(t, master, 1, 5, time.Now())
	}
	allExhausted := profileCall(t, client.Top, &papi.Empty{})
	if allExhausted.GetIsDailyPlayable() {
		t.Fatal("daily Top flag remains active after all three FREE allowances are exhausted")
	}
	for _, area := range allExhausted.GetDailyAreas() {
		for _, quest := range area.GetQuests() {
			if !quest.GetIsPlayable() || quest.GetPlayableCount() != 0 {
				t.Fatal("exhausting all FREE passes changed individual daily quest eligibility")
			}
		}
	}
	questSeedArea(t, selected, 1, 5, time.Now().Add(-24*time.Hour))
	top := profileCall(t, client.Top, &papi.Empty{})
	if !top.GetIsDailyPlayable() {
		t.Fatal("daily Top flag did not reactivate after one allowance refreshed")
	}
	for _, quest := range questFindArea(t, top.GetDailyAreas(), selected.GetId()).GetQuests() {
		if !quest.GetIsPlayable() || quest.GetPlayableCount() != 5 {
			t.Fatal("new JST date did not refresh daily free passes")
		}
	}
	storedAreas, err := opuser.FindAllUserAreas(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, stored := range storedAreas {
		if stored.GetAreaId() == selected.GetId() && (stored.GetDailyClearCount() != 0 || stored.GetClearQuestCount() != 1) {
			t.Fatal("daily reset was not persisted or erased permanent clear progress")
		}
	}
	get := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	for _, stored := range get.GetAreas() {
		if stored.GetAreaId() == selected.GetId() && stored.GetDailyClearCount() != 0 {
			t.Fatal("User.Get returned stale daily consumption")
		}
	}
	if userItemAmount(t, ticketID) != 7 || len(top.GetBulkDailyInfos()) != 0 {
		t.Fatal("listing consumed paid passes or fabricated bulk daily deck history")
	}

	// User.Get can be the first read after rollover. It must perform the same
	// persisted reset without requiring a preceding visit to Quest.Top.
	historicalTime := time.Now().Add(-48 * time.Hour).UnixMilli()
	staleArea := &ptransaction.UserArea{
		AreaId: selected.GetId(), ClearQuestCount: 1, DailyClearCount: 5, LastClearedTime: historicalTime,
	}
	staleQuest := &ptransaction.UserQuest{
		QuestId: selected.GetQuestIds()[0], DailyClearCount: 5, LastClearedTime: historicalTime,
		HighestScore: 123456789, HighestScoreTime: historicalTime - 1000, HighestRank: 1,
		HighestScoreRank: penum.ResultRankType_ResultRankType_Splus,
	}
	if err := opuser.UpsertUserArea(ctx, testConfig.TestUserSuid, staleArea); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserQuest(ctx, testConfig.TestUserSuid, staleQuest); err != nil {
		t.Fatal(err)
	}
	expectedArea := proto.Clone(staleArea).(*ptransaction.UserArea)
	expectedQuest := proto.Clone(staleQuest).(*ptransaction.UserQuest)
	expectedArea.DailyClearCount, expectedQuest.DailyClearCount = 0, 0
	get = profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	foundArea, foundQuest := false, false
	for _, value := range get.GetAreas() {
		if value.GetAreaId() == expectedArea.GetAreaId() {
			foundArea = true
			if !proto.Equal(value, expectedArea) {
				t.Fatal("direct User.Get did not reset only the stale area counter")
			}
		}
	}
	for _, value := range get.GetQuests() {
		if value.GetQuestId() == expectedQuest.GetQuestId() {
			foundQuest = true
			if !proto.Equal(value, expectedQuest) {
				t.Fatal("direct User.Get reset changed historical quest score, rank, or timestamps")
			}
		}
	}
	if !foundArea || !foundQuest {
		t.Fatal("direct User.Get omitted existing daily progress during rollover")
	}
	storedQuest, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, expectedQuest.GetQuestId())
	if err != nil || !proto.Equal(storedQuest, expectedQuest) {
		t.Fatalf("direct User.Get quest rollover was not persisted: %v", err)
	}
	storedAreas, err = opuser.FindAllUserAreas(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range storedAreas {
		if value.GetAreaId() == expectedArea.GetAreaId() && !proto.Equal(value, expectedArea) {
			t.Fatal("direct User.Get area rollover was not persisted")
		}
	}
	if userItemAmount(t, ticketID) != 7 {
		t.Fatal("direct User.Get rollover consumed paid passes")
	}
}

func TestQuestTowerProgressionScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewQuestClient(testConfig.GrpcConn)
	areas, _ := questCatalogFixture(t)
	towers := questAreasOfType(areas, penum.AreaType_AreaType_Tower)
	if len(towers) != 1 || len(towers[0].GetQuestIds()) < 6 {
		t.Fatal("master primary tower needs at least six stages")
	}
	tower := towers[0]
	assertPage := func(area *papi.QuestAreaInfo, count, current int) {
		t.Helper()
		if area.GetAreaId() != tower.GetId() || len(area.GetQuests()) != count {
			t.Fatalf("tower visible page length = %d, want %d", len(area.GetQuests()), count)
		}
		for i, quest := range area.GetQuests() {
			if quest.GetQuestId() != tower.GetQuestIds()[i] || quest.GetIsPlayable() != (i == current) || quest.GetCleared() != (i < current) {
				t.Fatalf("tower progression is not sequential at %s", quest.GetQuestId())
			}
			assertQuestNoScore(t, quest)
		}
	}
	first := profileCall(t, client.ListTower, &papi.Empty{})
	assertPage(first.GetArea(), 5, 0)
	for i := range 5 {
		questSeedCleared(t, tower.GetQuestIds()[i])
	}
	questSeedArea(t, tower, 5, 0, time.Now())
	second := profileCall(t, client.ListTower, &papi.Empty{})
	assertPage(second.GetArea(), min(10, len(tower.GetQuestIds())), 5)
	top := profileCall(t, client.Top, &papi.Empty{})
	towerTop := profileCall(t, client.TowerTop, &papi.Empty{})
	if top.GetCurrentTowerQuestId() != tower.GetQuestIds()[5] || towerTop.GetTowerArea().GetCurrentTowerQuestId() != tower.GetQuestIds()[5] || !proto.Equal(second.GetArea(), towerTop.GetTowerArea().GetArea()) {
		t.Fatal("Top and TowerTop disagree with persisted tower progression")
	}
	if top.GetIsTowerTopUnlocked() || len(towerTop.GetSubTowerAreas()) != 0 {
		t.Fatal("early tower progress prematurely unlocked subtowers")
	}
}

func TestQuestGodCatalogScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	client := papi.NewQuestClient(testConfig.GrpcConn)
	areas, _ := questCatalogFixture(t)
	top := profileCall(t, client.Top, &papi.Empty{})
	assertQuestOfflineTop(t, top)
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	for _, master := range main {
		area := questFindMainArea(t, top.GetMainAreaGroups(), master.GetId())
		if !area.GetUnlocked() || len(area.GetQuests()) != len(master.GetQuestIds()) {
			t.Fatalf("god main area is incomplete or locked: %s", master.GetId())
		}
		for _, quest := range area.GetQuests() {
			if !quest.GetCleared() || quest.GetIsPlayable() {
				t.Fatalf("god main quest is uncleared or replayable: %s", quest.GetQuestId())
			}
			assertQuestNoScore(t, quest)
		}
	}
	daily := questPermanentDailyAreas(areas)
	assertQuestDailyAreas(t, top.GetDailyAreas(), daily)
	for _, master := range daily {
		area := questFindArea(t, top.GetDailyAreas(), master.GetId())
		if !area.GetUnlocked() || len(area.GetQuests()) != len(master.GetQuestIds()) {
			t.Fatalf("god daily area is incomplete or locked: %s", master.GetId())
		}
		for _, quest := range area.GetQuests() {
			if !quest.GetCleared() || !quest.GetIsPlayable() || quest.GetPlayableCount() != 5 {
				t.Fatalf("god daily quest is not cleared and repeatable: %s", quest.GetQuestId())
			}
		}
	}
	towerTop := profileCall(t, client.TowerTop, &papi.Empty{})
	towerInfos := append([]*papi.QuestTowerAreaInfo{towerTop.GetTowerArea()}, towerTop.GetSubTowerAreas()...)
	for _, info := range towerInfos {
		if !info.GetArea().GetUnlocked() {
			t.Fatalf("god tower area remains locked: %s", info.GetArea().GetAreaId())
		}
		for _, quest := range info.GetArea().GetQuests() {
			if !quest.GetCleared() || quest.GetIsPlayable() {
				t.Fatalf("god tower quest is uncleared or replayable: %s", quest.GetQuestId())
			}
			assertQuestNoScore(t, quest)
		}
	}
	home := profileCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	if home.GetCurrentMainAreaId() != main[len(main)-1].GetId() || home.GetCurrentTowerQuestId() != top.GetCurrentTowerQuestId() {
		t.Fatal("Home current live IDs disagree with god quest catalog")
	}
	get := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	cleared := make(map[string]bool)
	for _, quest := range get.GetQuests() {
		cleared[quest.GetQuestId()] = quest.GetLastClearedTime() > 0
	}
	for _, master := range append(append(main, daily...), questAreasOfType(areas, penum.AreaType_AreaType_Tower)...) {
		for _, id := range master.GetQuestIds() {
			if !cleared[id] {
				t.Fatalf("User.Get omitted god-cleared quest %s", id)
			}
		}
	}
	// Admin-cleared quests do not represent today's played attempts. Repeating
	// god mode must refresh exhausted allowances without losing clear history.
	ctx := context.Background()
	for _, master := range daily {
		questSeedArea(t, master, int32(len(master.GetQuestIds())), 5, time.Now())
		quest, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, master.GetQuestIds()[0])
		if err != nil {
			t.Fatal(err)
		}
		quest.DailyClearCount = 5
		if err := opuser.UpsertUserQuest(ctx, testConfig.TestUserSuid, quest); err != nil {
			t.Fatal(err)
		}
	}
	promoteTestUser(t)
	refreshed := profileCall(t, client.Top, &papi.Empty{})
	if !refreshed.GetIsDailyPlayable() {
		t.Fatal("repeated god promotion did not restore daily free allowances")
	}
	for _, area := range refreshed.GetDailyAreas() {
		for _, quest := range area.GetQuests() {
			if !quest.GetCleared() || !quest.GetIsPlayable() || quest.GetPlayableCount() != 5 {
				t.Fatalf("repeated god promotion consumed passes or lost clear progress: %s", quest.GetQuestId())
			}
		}
	}
	get = profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	for _, quest := range get.GetQuests() {
		if cleared[quest.GetQuestId()] && quest.GetLastClearedTime() <= 0 {
			t.Fatalf("repeated god promotion removed clear history for %s", quest.GetQuestId())
		}
		for _, area := range daily {
			if slices.Contains(area.GetQuestIds(), quest.GetQuestId()) && quest.GetDailyClearCount() != 0 {
				t.Fatalf("repeated god promotion left a consumed daily quest counter: %s", quest.GetQuestId())
			}
		}
	}
	for _, area := range get.GetAreas() {
		for _, master := range daily {
			if area.GetAreaId() == master.GetId() && area.GetDailyClearCount() != 0 {
				t.Fatalf("repeated god promotion left a consumed daily area counter: %s", area.GetAreaId())
			}
		}
	}
}

func questCatalogFixture(t *testing.T) ([]*pmaster.Area, map[string]*pmaster.Quest) {
	t.Helper()
	areas, err := opmaster.FindAllAreas(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	quests, err := opmaster.FindAllQuests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]*pmaster.Quest, len(quests))
	for _, quest := range quests {
		byID[quest.GetId()] = quest
	}
	return areas, byID
}

func questAreasOfType(areas []*pmaster.Area, kind penum.AreaType) []*pmaster.Area {
	result := make([]*pmaster.Area, 0)
	for _, area := range areas {
		if area.GetType() == kind {
			result = append(result, area)
		}
	}
	slices.SortFunc(result, func(a, b *pmaster.Area) int { return int(a.GetOrder() - b.GetOrder()) })
	return result
}

func questPermanentDailyAreas(areas []*pmaster.Area) []*pmaster.Area {
	return slices.DeleteFunc(questAreasOfType(areas, penum.AreaType_AreaType_Daily), func(area *pmaster.Area) bool {
		return area.GetDailySpecialTicketItemId() != ""
	})
}

func questFindMainArea(t *testing.T, groups []*papi.QuestAreaGroupInfo, id string) *papi.QuestAreaInfo {
	t.Helper()
	var areas []*papi.QuestAreaInfo
	for _, group := range groups {
		areas = append(areas, group.GetAreas()...)
	}
	return questFindArea(t, areas, id)
}

func questFindArea(t *testing.T, areas []*papi.QuestAreaInfo, id string) *papi.QuestAreaInfo {
	t.Helper()
	for _, area := range areas {
		if area.GetAreaId() == id {
			return area
		}
	}
	t.Fatalf("quest response omitted area %s", id)
	return nil
}

func questSeedCleared(t *testing.T, id string) {
	t.Helper()
	// A nonzero stored score ensures main/tower list presentation does not
	// accidentally expose score/rank fields reserved for repeatable modes.
	if err := opuser.UpsertUserQuest(context.Background(), testConfig.TestUserSuid, &ptransaction.UserQuest{
		QuestId: id, LastClearedTime: time.Now().UnixMilli(), HighestScore: 123456789,
		HighestScoreRank: penum.ResultRankType_ResultRankType_Splus,
	}); err != nil {
		t.Fatal(err)
	}
}

func questSeedArea(t *testing.T, area *pmaster.Area, clearCount, dailyCount int32, last time.Time) {
	t.Helper()
	if err := opuser.UpsertUserArea(context.Background(), testConfig.TestUserSuid, &ptransaction.UserArea{
		AreaId: area.GetId(), ClearQuestCount: clearCount, DailyClearCount: dailyCount, LastClearedTime: last.UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
}

func questSeedFullArea(t *testing.T, area *pmaster.Area) {
	t.Helper()
	for _, id := range area.GetQuestIds() {
		questSeedCleared(t, id)
	}
	questSeedArea(t, area, int32(len(area.GetQuestIds())), 0, time.Now())
}

func questSetManagerLevel(t *testing.T, level int32) {
	t.Helper()
	ctx := context.Background()
	master, err := opmaster.FindManagerLevel(ctx, level)
	if err != nil {
		t.Fatal(err)
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	user.ManagerLevel, user.ManagerExp = level, master.GetRequiredExp()
	if err := opuser.UpsertUser(ctx, testConfig.TestUserSuid, user); err != nil {
		t.Fatal(err)
	}
}

func assertQuestNoScore(t *testing.T, quest *papi.QuestInfo) {
	t.Helper()
	if quest.GetHighestScore() != 0 || quest.GetRankType() != 0 || quest.GetPlus() != 0 || quest.GetHighestRank() != 0 {
		t.Fatalf("nonrepeatable quest unexpectedly exposes score/rank: %s", quest.GetQuestId())
	}
}

func assertQuestOfflineTop(t *testing.T, top *papi.QuestTopResponse) {
	t.Helper()
	if top.GetPvpInfo().GetLatestPvpSeason() != nil || top.GetGvgInfo() != nil || len(top.GetBulkDailyInfos()) != 0 {
		t.Fatal("quest top advertised multiplayer events or fabricated bulk live history")
	}
}

func assertQuestDailyAreas(t *testing.T, got []*papi.QuestAreaInfo, want []*pmaster.Area) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("daily catalog contains %d areas, want %d permanent types", len(got), len(want))
	}
	for i, area := range got {
		if area.GetAreaId() != want[i].GetId() {
			t.Fatalf("daily catalog included a special area or changed master order: %s", area.GetAreaId())
		}
	}
}
