package test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

func TestLoveEntranceSelectionScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewLoveClient(testConfig.GrpcConn)
	homeClient := papi.NewHomeClient(testConfig.GrpcConn)
	title := loveCall(t, client.Title, &papi.Empty{})
	assertLoveStandardTitle(t, title)
	for _, info := range title.GetLoveInfos() {
		if !info.GetIsUnplayed() {
			t.Fatal("a fresh Title registered a standard event")
		}
	}
	assertHomeLoveEntrances(t, loveCall(t, homeClient.Enter, &papi.HomeEnterRequest{}))
	unlockTestLove(t)
	// Keep most standard events unregistered while giving the first one
	// distinct progress flags. A randomly selected banner must use its own
	// state rather than copy the first series' registration/reward/item flags.
	first := title.GetLoveInfos()[0].GetLoveId()
	loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: first, Name: "Manager"})
	catalog, err := moshikoi.LoadCatalog(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	state := love2024State(t, first)
	state.LastLessonRewardTime = time.Now().Add(-48 * time.Hour).UnixMilli()
	storeLoveState(t, state)
	if catalog.Config.PresentItemID != "" {
		setLoveItem(t, catalog.Config.PresentItemID, 1)
	}
	for _, id := range moshikoi.SupportedLoveIDs() {
		cfg, err := moshikoi.LoadConfig(id)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.BaseType != penum.LoveBaseType_LoveBaseType_Extra {
			continue
		}
		loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: id, Name: "Manager"})
		state := love2024State(t, id)
		state.LastLessonRewardTime = time.Now().UnixMilli()
		storeLoveState(t, state)
		extra := loveCall(t, client.TitleExtra, &papi.LoveTitleExtraRequest{LoveId: id})
		if extra.GetLoveInfo().GetLoveId() != id || extra.GetLoveInfo().GetBaseType() != penum.LoveBaseType_LoveBaseType_Extra || extra.GetLoveInfo().GetIsUnplayed() {
			t.Fatalf("dedicated Extra title lost its event: %v", extra)
		}
	}
	for range 12 {
		// Randomness is checked by invariants, never by requiring every series
		// to appear in a finite sample (which would make the test flaky).
		assertHomeLoveEntrances(t, loveCall(t, homeClient.Enter, &papi.HomeEnterRequest{}))
	}
	title = loveCall(t, client.Title, &papi.Empty{})
	assertLoveStandardTitle(t, title)
	for _, info := range title.GetLoveInfos() {
		if info.GetIsUnplayed() != (info.GetLoveId() != first) || !info.GetIsUnlocked() {
			t.Fatalf("Title dropped or changed a standard series based on the random Home banner: %v", info)
		}
	}
	if top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: first}); top.GetPlayerName() != "Manager" {
		t.Fatal("the registered standard title was unavailable independently of the selected Home banner")
	}
}

func TestLoveHiddenSeriesDailyReconciliationScenario(t *testing.T) {
	setupTestUser(t)
	unlockTestLove(t)
	ctx := context.Background()
	client := papi.NewLoveClient(testConfig.GrpcConn)
	masters, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	type dailyPair struct{ loveID, loginID, activityID string }
	var pairs []dailyPair
	for _, id := range moshikoi.SupportedLoveIDs() {
		loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: id, Name: "Manager"})
		cfg, err := moshikoi.LoadConfig(id)
		if err != nil {
			t.Fatal(err)
		}
		pair := dailyPair{loveID: id}
		for _, master := range masters {
			if !slices.Contains(cfg.LimitedEventMissionIDs, master.GetEventMissionId()) || master.GetCategoryType() != penum.MissionCategoryType_MissionCategoryType_Daily {
				continue
			}
			switch master.GetMissionType() {
			case penum.MissionType_MissionType_LoginCount:
				pair.loginID = master.GetId()
			case penum.MissionType_MissionType_CharacterStartActivityFanEventCount,
				penum.MissionType_MissionType_PhotoExpressionShootingSpecifiedCharacterShootCount:
				// Extra currently uses a daily photo task instead of a fan event.
				pair.activityID = master.GetId()
			}
		}
		if pair.loginID == "" || pair.activityID == "" {
			t.Fatalf("%s master lacks the daily login/activity fixture", id)
		}
		pairs = append(pairs, pair)
	}
	// Seed only the configured test user's yesterday-claimed missions. This
	// deliberately happens after all registrations, so no earlier RPC refresh
	// can mask Home accidentally reconciling only its chosen banner.
	for _, pair := range pairs {
		for _, id := range []string{pair.loginID, pair.activityID} {
			if err := opuser.UpsertUserMission(ctx, testConfig.TestUserSuid, &ptransaction.UserMission{
				MissionId: id, CurrentThreshold: 1, Progress: 99, StatusType: penum.MissionStatusType_MissionStatusType_Received,
				LastUpdateTime: time.Now().Add(-48 * time.Hour).UnixMilli(),
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	started := time.Now().UnixMilli()
	home := loveCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	selected := assertHomeLoveEntrances(t, home)
	stored, err := opuser.FindAllUserMissions(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	hiddenStandards := 0
	for _, pair := range pairs {
		cfg, err := moshikoi.LoadConfig(pair.loveID)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.BaseType == penum.LoveBaseType_LoveBaseType_Standard && pair.loveID != selected {
			hiddenStandards++
		}
		login, activity := findUserMissionByID(stored, pair.loginID), findUserMissionByID(stored, pair.activityID)
		if login == nil || activity == nil || login.GetProgress() != 1 || activity.GetProgress() != 0 || activity.GetStatusType() != penum.MissionStatusType_MissionStatusType_Progress ||
			login.GetLastUpdateTime() < started || activity.GetLastUpdateTime() < started {
			t.Fatalf("Home omitted the daily refresh for series %s (selected=%s): login=%v activity=%v", pair.loveID, selected, login, activity)
		}
		for _, mission := range []*ptransaction.UserMission{login, activity} {
			if !proto.Equal(mission, findUserMissionByID(home.GetCommonResponse().GetUpdatedData().GetMissions(), mission.GetMissionId())) {
				t.Fatalf("Home omitted the persisted mission delta for %s", mission.GetMissionId())
			}
		}
	}
	if hiddenStandards == 0 {
		t.Fatal("fixture did not cover any standard series hidden by banner selection")
	}
	repeated := loveCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	assertHomeLoveEntrances(t, repeated)
	afterRepeat, err := opuser.FindAllUserMissions(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range pairs {
		for _, id := range []string{pair.loginID, pair.activityID} {
			if !proto.Equal(findUserMissionByID(stored, id), findUserMissionByID(afterRepeat, id)) {
				t.Fatalf("changing the Home banner reset or credited daily mission %s twice", id)
			}
		}
		// Every registered series remains accessible regardless of visibility.
		if top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: pair.loveID}); top.GetPlayerName() != "Manager" {
			t.Fatalf("series %s became inaccessible when its Home banner was hidden", pair.loveID)
		}
	}
}

func assertLoveStandardTitle(t *testing.T, title *papi.LoveTitleResponse) {
	t.Helper()
	var expected []string
	for _, id := range moshikoi.SupportedLoveIDs() {
		cfg, err := moshikoi.LoadConfig(id)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.BaseType == penum.LoveBaseType_LoveBaseType_Standard {
			expected = append(expected, id)
		}
	}
	if len(title.GetLoveInfos()) != len(expected) {
		t.Fatalf("Love.Title returned %d events; want all %d Standards and no Extras", len(title.GetLoveInfos()), len(expected))
	}
	for index, info := range title.GetLoveInfos() {
		if info.GetLoveId() != expected[index] || info.GetBaseType() != penum.LoveBaseType_LoveBaseType_Standard || info.GetEndTime() != 0 {
			t.Fatalf("Love.Title lost standard ordering or included an Extra: %v", info)
		}
	}
}

func assertHomeLoveEntrances(t *testing.T, home *papi.HomeEnterResponse) string {
	t.Helper()
	ctx, suid := context.Background(), testConfig.TestUserSuid
	catalogs := make(map[string]*moshikoi.Catalog)
	extraCount, standardCount := 0, 0
	for _, id := range moshikoi.SupportedLoveIDs() {
		catalog, err := moshikoi.LoadCatalog(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		catalogs[id] = catalog
		if catalog.Config.BaseType == penum.LoveBaseType_LoveBaseType_Extra {
			extraCount++
		} else {
			standardCount++
		}
	}
	wantCount := extraCount
	if standardCount > 0 {
		wantCount++
	}
	entries := home.GetEventInfo().GetLoveEventInfos()
	if len(entries) != wantCount {
		t.Fatalf("Home returned %d Love entrances; want one Standard plus %d Extras", len(entries), extraCount)
	}
	// Check unlock flags against the dedicated title endpoint, but registration,
	// lesson and inventory flags against the selected event's stored state.
	client := papi.NewLoveClient(testConfig.GrpcConn)
	title := loveCall(t, client.Title, &papi.Empty{})
	assertLoveStandardTitle(t, title)
	infos := make(map[string]*papi.LoveInfo)
	for _, info := range title.GetLoveInfos() {
		infos[info.GetLoveId()] = info
	}
	items, err := opuser.FindAllUserItems(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	seen, selected := map[string]bool{}, ""
	for _, entry := range entries {
		catalog := catalogs[entry.GetId()]
		if catalog == nil || seen[entry.GetId()] {
			t.Fatalf("Home returned an unknown or duplicate Love entrance: %v", entry)
		}
		seen[entry.GetId()] = true
		cfg := catalog.Config
		wantTitleAsset := ""
		if cfg.BaseType == penum.LoveBaseType_LoveBaseType_Extra {
			wantTitleAsset = catalog.TitleAssetID
			infos[entry.GetId()] = loveCall(t, client.TitleExtra, &papi.LoveTitleExtraRequest{LoveId: entry.GetId()}).GetLoveInfo()
		} else {
			if selected != "" {
				t.Fatal("Home returned more than one standard Love entrance")
			}
			selected = entry.GetId()
		}
		state, err := moshikoi.LoadState(ctx, suid, entry.GetId())
		if err != nil {
			t.Fatal(err)
		}
		hasPresent, now := false, time.Now().UnixMilli()
		for _, item := range items {
			if cfg.PresentItemID != "" && item.GetItemId() == cfg.PresentItemID && item.GetAmount() > 0 && (item.GetExpiredTime() == 0 || item.GetExpiredTime() > now) {
				hasPresent = true
			}
		}
		if entry.GetBaseType() != cfg.BaseType || entry.GetName() != cfg.Name || entry.GetStartTime() != cfg.StartTime || entry.GetEndTime() != 0 || entry.GetBannerAssetId() != cfg.BannerAssetID ||
			entry.GetLimitedExtraTitleAssetId() != wantTitleAsset || entry.GetUnlockConditionId() != cfg.UnlockConditionID || entry.GetIsUnlocked() != infos[entry.GetId()].GetIsUnlocked() ||
			entry.GetIsUnplayed() == state.Registered || entry.GetHasHiddenFavorabilityObtainItem() != hasPresent ||
			!slices.Equal(entry.GetEventMissionIds(), cfg.EventMissionIDs) || !slices.Equal(entry.GetLimitedEventMissionIds(), cfg.LimitedEventMissionIDs) {
			t.Fatalf("Home entrance %s mixed another series' metadata or progress: %v", entry.GetId(), entry)
		}
		maxRewardTime := catalog.LessonProgress(state).GetMaxRewardTime()
		// Avoid treating crossing the exact reward-window boundary between the
		// response and this database check as a server defect.
		if maxRewardTime < now-2000 || maxRewardTime > now+2000 {
			if entry.GetIsLessonRewardMax() != (state.Registered && now >= maxRewardTime) {
				t.Fatalf("Home entrance %s used another series' lesson timer", entry.GetId())
			}
		}
	}
	if standardCount > 0 && selected == "" {
		t.Fatal("Home omitted its standard Love entrance")
	}
	for id, catalog := range catalogs {
		if catalog.Config.BaseType == penum.LoveBaseType_LoveBaseType_Extra && !seen[id] {
			t.Fatalf("Home omitted Extra entrance %s", id)
		}
	}
	return selected
}
