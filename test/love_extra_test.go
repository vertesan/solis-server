package test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

const testExtraLoveID = "love-extra-25-0426"

type extraLoveEpisode struct {
	suffix  string
	choices []int32
}

func TestLoveExtraAllEndingRoutesScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerExtraLove(t)
	missionClient := papi.NewMissionClient(testConfig.GrpcConn)
	emblemEndings := map[string]int{"succubus-rui-happy": 1, "succubus-chs-happy": 2, "magical-smr-happy": 3, "magical-ngs-happy": 4, "succubus-rui-harem-happy": 5}
	for number := 1; number <= 5; number++ {
		loveReject(t, codes.FailedPrecondition, missionClient.Receive, &papi.MissionReceiveRequest{MissionIds: []string{fmt.Sprintf("event-love-25-0426-limited-mission_love-end_emb-sp-%02d", number)}})
	}
	cases := []struct {
		ending, terminal, name string
		path                   []extraLoveEpisode
		choices                []int32
	}{
		{"succubus-rui-happy", "005-succubus-rui-good", "ラブラブ大魔王END", extraRuiPath(false, false), nil},
		{"succubus-rui-bad", "005-succubus-rui-bad", "悪魔召喚END", extraRuiPath(true, false), nil},
		{"succubus-rui-harem-happy", "005-happy-end", "ドキドキ取り合いEND", extraRuiPath(true, true), nil},
		{"succubus-rui-harem-bad-01", "005-bad-end", "一人ぼっちの日常END", extraRuiPath(false, true), []int32{1}},
		{"succubus-rui-harem-bad-02", "005-bad-end", "ボンバーEND", extraRuiPath(false, true), []int32{2}},
		{"succubus-rui-harem-bad-03", "005-bad-end", "ビンタ×４END", extraRuiPath(false, true), []int32{3}},
		{"succubus-chs-happy", "005-succubus-chs-good", "人間さんと仲良しEND", extraChisaPath(true, true), nil},
		{"succubus-chs-bad", "005-succubus-chs-bad", "忘却END", extraChisaPath(false, true), nil},
		{"magical-smr-happy", "005-magical-smr-good", "解けない魔法END", extraSumirePath(true, false), []int32{1}},
		{"magical-smr-bad", "005-magical-smr-bad", "新しい関係END", extraSumirePath(true, true), nil},
		{"magical-ngs-happy", "005-magical-ngs-good", "魔法少女にゃぎさEND", extraNagisaPath(true), []int32{1, 1}},
		{"magical-ngs-bad", "005-magical-ngs-bad", "鈴の音END", extraNagisaPath(false), nil},
	}
	for index, tc := range cases {
		if index > 0 {
			reset := loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testExtraLoveID})
			if !slices.Equal(reset.GetStoryEpisodeIds(), []string{extraLoveEpisodeID("001")}) {
				t.Fatal("Extra reset confused the public event ID with the story ID namespace")
			}
		}
		playExtraLovePath(t, client, catalog, tc.path)
		top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testExtraLoveID})
		if !slices.Contains(top.GetStoryEpisodeIds(), extraLoveEpisodeID(tc.terminal)) {
			t.Fatalf("Extra %s reached the wrong terminal: %v", tc.ending, top.GetStoryEpisodeIds())
		}
		assertExtraLoveHeroine(t, top.GetMainCharacterInfo())
		finish := playExtraLoveEpisode(t, client, catalog, tc.terminal, tc.choices...)
		stored := storedLove2024(t, testExtraLoveID)
		if !slices.Contains(stored.GetUnlockedLoveStoryEndIDs(), "love-end-love-25-0426-005-"+tc.ending) || len(stored.GetUnlockedLoveStoryEndIDs()) != index+1 ||
			finish.GetStoryEndName() != tc.name || finish.GetUnlockSave() != (index == 0) || stored.GetHiddenFavorability() != 0 {
			t.Fatalf("Extra %s did not persist its named ending without a permanent favorability award: finish=%v stored=%v", tc.ending, finish, stored)
		}
		if number := emblemEndings[tc.ending]; number > 0 {
			missionID := fmt.Sprintf("event-love-25-0426-limited-mission_love-end_emb-sp-%02d", number)
			emblemID := fmt.Sprintf("emb-love-25-0426-true-%02d", number)
			claim := loveCall(t, missionClient.Receive, &papi.MissionReceiveRequest{MissionIds: []string{missionID}})
			if !slices.ContainsFunc(claim.GetResults(), func(reward *ptransaction.RewardResult) bool {
				return reward.GetResourceType() == penum.ResourceType_ResourceType_Emblem && reward.GetResourceId() == emblemID && reward.GetAmountI64() == 1
			}) {
				t.Fatalf("Extra %s did not award its own good-ending emblem: %v", tc.ending, claim)
			}
			if _, err := opuser.FindUserEmblem(context.Background(), testConfig.TestUserSuid, emblemID); err != nil {
				t.Fatalf("Extra good-ending emblem %s was not persisted: %v", emblemID, err)
			}
			loveReject(t, codes.FailedPrecondition, missionClient.Receive, &papi.MissionReceiveRequest{MissionIds: []string{missionID}})
		}
	}
	// Alternative Sumire/Nagisa responses produce the same ending. Replay all
	// paid chapters with no points after all twelve endings have been reached.
	setLoveItem(t, catalog.Config.StoryReadItemID, 0)
	before := proto.Clone(storedLove2024(t, testExtraLoveID)).(*ptransaction.UserLove)
	for _, replay := range []struct {
		path     []extraLoveEpisode
		terminal string
		choices  []int32
	}{
		{extraSumirePath(true, false), "005-magical-smr-good", []int32{2}},
		{extraNagisaPath(true), "005-magical-ngs-good", []int32{2, 2}},
	} {
		loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testExtraLoveID})
		playExtraLovePath(t, client, catalog, replay.path)
		finish := playExtraLoveEpisode(t, client, catalog, replay.terminal, replay.choices...)
		if finish.GetUnlockSave() || !proto.Equal(before, storedLove2024(t, testExtraLoveID)) || loveItemAmount(t, catalog.Config.StoryReadItemID) != 0 {
			t.Fatal("Extra completed replay charged points or duplicated permanent ending progress")
		}
	}
	assertExtraLoveVisibility(t)
}

func TestLoveExtraRuiPatternRoutesScenario(t *testing.T) {
	routes, err := moshikoi.LoadRoutes(testExtraLoveID)
	if err != nil {
		t.Fatal(err)
	}
	// Both patterns require all eight choices. Mixing individually favorable
	// answers does not satisfy either conjunction. The captured private-server
	// policy sends unmatched patterns to Rui's bad finale, not a nameless reset.
	patternB := extraRuiPath(true, false)
	conditions := [][2]int{{0, 3}, {1, 3}, {2, 0}, {2, 1}, {2, 2}, {2, 4}, {3, 2}, {3, 3}}
	for mask := range 256 {
		for _, harem := range []bool{false, true} {
			path := extraRuiPath(false, harem)
			for bit, at := range conditions {
				if mask&(1<<bit) != 0 {
					path[at[0]].choices[at[1]] = patternB[at[0]].choices[at[1]]
				}
			}
			want := "005-succubus-rui-bad"
			if mask == 0 && !harem {
				want = "005-succubus-rui-good"
			} else if mask == 0 && harem {
				want = "005-bad-end"
			} else if mask == 255 && harem {
				want = "005-happy-end"
			}
			if got := resolveExtraLovePath(t, routes, path); got != extraLoveEpisodeID(want) {
				t.Fatalf("Rui mask=%08b harem=%t reached %s, want %s", mask, harem, got, want)
			}
		}
	}
	// Capture 026 selects the neither-pattern alternative in EP4 group3.
	for _, harem := range []bool{false, true} {
		path := extraRuiPath(false, harem)
		path[3].choices[2] = 2
		if got := resolveExtraLovePath(t, routes, path); got != extraLoveEpisodeID("005-succubus-rui-bad") {
			t.Fatalf("captured unmatched Rui choice did not fall back to the named bad finale: %s", got)
		}
	}
}

func TestLoveExtraHeroinePrerequisitesScenario(t *testing.T) {
	routes, err := moshikoi.LoadRoutes(testExtraLoveID)
	if err != nil {
		t.Fatal(err)
	}
	for _, prerequisite := range []bool{false, true} {
		for _, choice := range []bool{false, true} {
			want := "005-succubus-chs-bad"
			if prerequisite && choice {
				want = "005-succubus-chs-good"
			}
			if got := resolveExtraLovePath(t, routes, extraChisaPath(prerequisite, choice)); got != extraLoveEpisodeID(want) {
				t.Fatalf("Chisa prerequisite=%t followHeart=%t reached %s, want %s", prerequisite, choice, got, want)
			}
			want = "005-magical-smr-bad"
			if prerequisite && !choice {
				want = "005-magical-smr-good"
			}
			if got := resolveExtraLovePath(t, routes, extraSumirePath(prerequisite, choice)); got != extraLoveEpisodeID(want) {
				t.Fatalf("Sumire prerequisite=%t teachMagic=%t reached %s, want %s", prerequisite, choice, got, want)
			}
		}
	}
	for _, good := range []bool{false, true} {
		want := "005-magical-ngs-bad"
		if good {
			want = "005-magical-ngs-good"
		}
		if got := resolveExtraLovePath(t, routes, extraNagisaPath(good)); got != extraLoveEpisodeID(want) {
			t.Fatalf("Nagisa choice good=%t reached %s, want %s", good, got, want)
		}
	}
	for first := int32(1); first <= 2; first++ {
		for second := int32(1); second <= 2; second++ {
			id := extraLoveEpisodeID("005-magical-ngs-good")
			selected, err := routes.SelectChoices(id, love2024FinishRequest(id, first, second).GetSelectStoryChoiceInfos())
			if err != nil {
				t.Fatal(err)
			}
			_, ending, err := routes.Resolve(id, selected)
			if err != nil || ending != "love-end-love-25-0426-005-magical-ngs-happy" {
				t.Fatal("Nagisa's equivalent finale answers produced different endings")
			}
		}
	}
}

func TestLoveExtraConditionalChoicesAndFallbackScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerExtraLove(t)
	path := extraNagisaPath(true)
	playExtraLovePath(t, client, catalog, path[:2])
	id := extraLoveEpisodeID("003-magical")
	loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: id})
	before := love2024State(t, testExtraLoveID)
	loveReject(t, codes.InvalidArgument, client.Finish, love2024FinishRequest(id, 1, 1, 1, 1, 1, 1))
	loveReject(t, codes.InvalidArgument, client.Finish, love2024FinishRequest(id, 1, 1, 1, 2, 2))
	after := love2024State(t, testExtraLoveID)
	if !storedLove2024(t, testExtraLoveID).GetIsReadingStory() || !slices.Equal(before.Selections, after.Selections) || !slices.Equal(before.StoryEpisodeIDs, after.StoryEpisodeIDs) {
		t.Fatal("malformed conditional magical Finish changed the active route")
	}
	history := love2024FinishRequest(id, 1, 1, 1, 1, 1, 2)
	history.SelectStoryChoiceInfos[5].IsValid = false
	loveCall(t, client.Finish, history)
	if !slices.Contains(love2024State(t, testExtraLoveID).StoryEpisodeIDs, extraLoveEpisodeID("004-magical-ngs")) {
		t.Fatal("inactive historical Sumire choice changed the Nagisa branch")
	}
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testExtraLoveID})
	path = extraRuiPath(false, false)
	path[3].choices[2] = 2 // Neither pattern: captured 026/027 fallback.
	playExtraLovePath(t, client, catalog, path)
	stored := storedLove2024(t, testExtraLoveID)
	if len(stored.GetUnlockedLoveStoryEndIDs()) != 0 || !slices.Contains(love2024State(t, testExtraLoveID).StoryEpisodeIDs, extraLoveEpisodeID("005-succubus-rui-bad")) {
		t.Fatal("mixed Rui route reset or awarded its ending before the finale was read")
	}
	finish := playExtraLoveEpisode(t, client, catalog, "005-succubus-rui-bad")
	if finish.GetStoryEndName() != "悪魔召喚END" || len(storedLove2024(t, testExtraLoveID).GetUnlockedLoveStoryEndIDs()) != 1 {
		t.Fatal("captured Rui fallback did not award the named bad ending on finale completion")
	}
}

func TestLoveExtraSaveLoadHeroineIsolationScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerExtraLove(t)
	rei := loveCatalog(t)
	raiseTestLoveLevel(t, rei)
	lovePlayEpisode(t, client, "001", 2, 2)
	lovePlayEpisode(t, client, "001", 2, 1)
	loveCall(t, client.Save, &papi.LoveSaveRequest{LoveId: testLoveID, SlotNumber: 1})
	loveCall(t, client.UpdateSaveName, &papi.LoveUpdateSaveNameRequest{LoveId: testLoveID, SlotNumber: 1, Name: "Rei save"})
	reiBefore, reiLoveBefore := loveState(t), storedLove(t)
	playExtraLovePath(t, client, catalog, extraRuiPath(false, false))
	playExtraLoveEpisode(t, client, catalog, "005-succubus-rui-good")
	chisa := extraChisaPath(true, true)
	playExtraLovePath(t, client, catalog, chisa[:3])
	loveCall(t, client.Save, &papi.LoveSaveRequest{LoveId: testExtraLoveID, SlotNumber: 1})
	loveCall(t, client.UpdateSaveName, &papi.LoveUpdateSaveNameRequest{LoveId: testExtraLoveID, SlotNumber: 1, Name: "Chisa route"})
	saved := love2024State(t, testExtraLoveID)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testExtraLoveID})
	playExtraLovePath(t, client, catalog, extraSumirePath(true, false)[:3])
	pointsBefore := loveItemAmount(t, catalog.Config.StoryReadItemID)
	load := loveCall(t, client.Load, &papi.LoveLoadRequest{LoveId: testExtraLoveID, SlotNumber: 1})
	loaded := love2024State(t, testExtraLoveID)
	if !slices.Equal(load.GetStoryEpisodeIds(), saved.StoryEpisodeIDs) || !slices.Equal(loaded.Selections, saved.Selections) || loaded.Exp != saved.Exp ||
		loveItemAmount(t, catalog.Config.StoryReadItemID) != pointsBefore || !proto.Equal(loaded.Saves[0].Info, saved.Saves[0].Info) {
		t.Fatal("Extra Load did not restore the saved heroine route without rolling back resources")
	}
	playExtraLoveEpisode(t, client, catalog, chisa[3].suffix, chisa[3].choices...)
	finish := playExtraLoveEpisode(t, client, catalog, "005-succubus-chs-good")
	if finish.GetStoryEndName() != "人間さんと仲良しEND" {
		t.Fatal("restored Chisa route lost its episode1 prerequisite")
	}
	reiAfter := loveState(t)
	if !slices.Equal(reiBefore.Selections, reiAfter.Selections) || !slices.Equal(reiBefore.StoryEpisodeIDs, reiAfter.StoryEpisodeIDs) ||
		!proto.Equal(reiBefore.Saves[0].Info, reiAfter.Saves[0].Info) || !slices.Equal(reiBefore.Saves[0].Selections, reiAfter.Saves[0].Selections) ||
		!proto.Equal(reiLoveBefore, storedLove(t)) {
		t.Fatal("Extra's save slot or progress overwrote Rei's saved game")
	}
	assertExtraLoveVisibility(t)
}

func TestLoveExtraLessonAndUnavailablePresentScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerExtraLove(t)
	if catalog.Config.PresentItemID != "" {
		t.Fatal("Extra incorrectly advertises a nonexistent present item")
	}
	state := love2024State(t, testExtraLoveID)
	state.Exp, state.LastLessonRewardTime = 0, time.Now().UnixMilli()
	storeLoveState(t, state)
	setLoveItem(t, catalog.Config.StoryReadItemID, 0)
	setLoveItem(t, catalog.Config.PromoteItemID, 1)
	setLoveItem(t, loveCatalog(t).Config.PresentItemID, 3)
	itemsBefore, err := opuser.FindAllUserItems(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	loveBefore := storedLove2024(t, testExtraLoveID)
	// Story IDs use the content namespace, not the runtime "love-extra" ID.
	// They must still be protected from bypassing Love's route/cost checks.
	loveReject(t, codes.FailedPrecondition, papi.NewStoryClient(testConfig.GrpcConn).Read, &papi.StoryReadRequest{StoryId: "st-love-25-0426-002"})
	loveReject(t, codes.FailedPrecondition, client.Present, &papi.LovePresentRequest{LoveId: testExtraLoveID})
	itemsAfter, err := opuser.FindAllUserItems(context.Background(), testConfig.TestUserSuid)
	if err != nil || !proto.Equal(&papi.UpdatedData{Items: itemsBefore}, &papi.UpdatedData{Items: itemsAfter}) || !proto.Equal(loveBefore, storedLove2024(t, testExtraLoveID)) || love2024State(t, testExtraLoveID).Exp != state.Exp {
		t.Fatalf("rejected Extra present consumed another event's resources or changed progress: %v", err)
	}
	top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testExtraLoveID})
	if top.GetItemInfo().GetHiddenFavorabilityObtainItemId() != "" {
		t.Fatal("Extra Top exposed a present item despite this feature being unavailable")
	}
	assertDummyNoticeInfo(t, "Love.Top Extra", top.GetNoticeInfo())
	level := catalog.LevelData(state.Exp)
	steps := catalog.Config.PromoteMinutes * 60 / catalog.Config.LessonStepSeconds
	expectedExp := steps * level.GetLessonStepRewardExp()
	expectedPoints := steps * level.GetLessonStepRewardStoryReadItemAmount()
	drink := loveCall(t, client.PromoteLesson, &papi.LovePromoteLessonRequest{LoveId: testExtraLoveID})
	if drink.GetObtainExp() != expectedExp || drink.GetTotalExp() != expectedExp || love2024State(t, testExtraLoveID).Exp != expectedExp ||
		loveItemAmount(t, catalog.Config.PromoteItemID) != 0 || loveItemAmount(t, catalog.Config.StoryReadItemID) != expectedPoints {
		t.Fatalf("Extra skip drink did not consume one item and grant its master-defined EXP/story rewards: %v", drink)
	}
	if expectedPoints > 0 && !slices.ContainsFunc(drink.GetRewards(), func(reward *ptransaction.Reward) bool {
		return reward.GetResourceType() == penum.ResourceType_ResourceType_Item && reward.GetResourceId() == catalog.Config.StoryReadItemID && reward.GetAmount() == expectedPoints
	}) {
		t.Fatal("Extra skip-drink response omitted its persisted story-point reward")
	}
	loveReject(t, codes.ResourceExhausted, client.PromoteLesson, &papi.LovePromoteLessonRequest{LoveId: testExtraLoveID})
	if love2024State(t, testExtraLoveID).Exp != expectedExp || loveItemAmount(t, catalog.Config.StoryReadItemID) != expectedPoints {
		t.Fatal("Extra rejected skip drink still granted rewards")
	}
}

func extraRuiPath(patternB, harem bool) []extraLoveEpisode {
	path := []extraLoveEpisode{
		{"001", []int32{1, 1, 1, 1, 1}}, {"002", []int32{1, 1, 1, 1, 2}},
		{"003-succubus", []int32{2, 2, 1, 1, 1, 1}}, {"004-succubus-rui", []int32{1, 1, 1, 1, 1}},
	}
	if patternB {
		path[0].choices[3], path[1].choices[3] = 3, 2
		path[2].choices = []int32{1, 1, 3, 1, 3, 1}
		path[3].choices[2], path[3].choices[3] = 3, 2
	}
	if harem {
		path[3].choices[4] = 2
	}
	return path
}

func extraChisaPath(prerequisite, followHeart bool) []extraLoveEpisode {
	path := extraRuiPath(false, false)
	if prerequisite {
		path[0].choices[3] = 2
	}
	path[2].choices[5] = 2
	path[3] = extraLoveEpisode{"004-succubus-chs", []int32{1, 1, 1, 1, 1}}
	if followHeart {
		path[3].choices[4] = 2
	}
	return path
}

func extraSumirePath(prerequisite, teachMagic bool) []extraLoveEpisode {
	path := extraRuiPath(false, false)
	path[1].choices[4] = 1
	path[2] = extraLoveEpisode{"003-magical", []int32{1, 1, 1, 1, 2, 1}}
	if prerequisite {
		path[2].choices[3] = 2
	}
	path[3] = extraLoveEpisode{"004-magical-smr", []int32{1, 1, 1, 1, 1, 1, 1, 1, 1}}
	if teachMagic {
		path[3].choices[8] = 2
	}
	return path
}

func extraNagisaPath(good bool) []extraLoveEpisode {
	path := extraSumirePath(false, false)
	path[2].choices = []int32{1, 1, 1, 1, 1}
	path[3] = extraLoveEpisode{"004-magical-ngs", []int32{1, 1, 1, 1, 1}}
	if good {
		path[3].choices[4] = 2
	}
	return path
}

func extraLoveEpisodeID(suffix string) string { return "love-event-story-25-0426-" + suffix }

func registerExtraLove(t *testing.T) (papi.LoveClient, *moshikoi.Catalog) {
	t.Helper()
	client, _ := registerTestLove(t)
	loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: testExtraLoveID, Name: "Manager"})
	return client, prepareLove2024Series(t, testExtraLoveID)
}

func playExtraLovePath(t *testing.T, client papi.LoveClient, catalog *moshikoi.Catalog, path []extraLoveEpisode) {
	t.Helper()
	for _, episode := range path {
		playExtraLoveEpisode(t, client, catalog, episode.suffix, episode.choices...)
	}
}

func playExtraLoveEpisode(t *testing.T, client papi.LoveClient, catalog *moshikoi.Catalog, suffix string, choices ...int32) *papi.LoveFinishResponse {
	t.Helper()
	id := extraLoveEpisodeID(suffix)
	master := catalog.Episode(id)
	if master == nil {
		t.Fatalf("Extra catalog omitted master episode %s", id)
	}
	before, cost := loveItemAmount(t, catalog.Config.StoryReadItemID), int64(master.GetConsumptionItemAmount())
	if len(storedLove2024(t, testExtraLoveID).GetUnlockedLoveStoryEndIDs()) == 12 {
		cost = 0
	}
	loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: id})
	if loveItemAmount(t, catalog.Config.StoryReadItemID) != before-cost {
		t.Fatalf("Extra %s did not charge its master story price %d", suffix, cost)
	}
	finish := loveCall(t, client.Finish, love2024FinishRequest(id, choices...))
	want := moshikoi.VisibleLove(storedLove2024(t, testExtraLoveID))
	if !slices.ContainsFunc(finish.GetCommonResponse().GetUpdatedData().GetLoves(), func(love *ptransaction.UserLove) bool { return proto.Equal(love, want) && !love.GetIsReadingStory() }) {
		t.Fatalf("Extra %s Finish omitted its event-keyed persisted state", suffix)
	}
	return finish
}

func resolveExtraLovePath(t *testing.T, routes *moshikoi.Routes, path []extraLoveEpisode) string {
	t.Helper()
	var selections []umodel.LoveStorySelection
	var next string
	for index, episode := range path {
		id := extraLoveEpisodeID(episode.suffix)
		selected, err := routes.SelectChoices(id, love2024FinishRequest(id, episode.choices...).GetSelectStoryChoiceInfos())
		if err != nil {
			t.Fatalf("Extra %s rejected valid selections: %v", episode.suffix, err)
		}
		selections = append(selections, selected...)
		var ending string
		next, ending, err = routes.Resolve(id, selections)
		if err != nil || ending != "" || next == "" {
			t.Fatalf("Extra %s terminated before its finale: next=%s ending=%s err=%v", episode.suffix, next, ending, err)
		}
		if index < len(path)-1 && next != extraLoveEpisodeID(path[index+1].suffix) {
			t.Fatalf("Extra %s reached %s instead of the next chapter %s", episode.suffix, next, path[index+1].suffix)
		}
	}
	return next
}

func assertExtraLoveHeroine(t *testing.T, info *papi.LoveMainCharacterInfo) {
	t.Helper()
	wantCostumes := map[string]string{"char-rui": "cos-rui-sucu-00", "char-chs": "cos-chs-sucu-00", "char-smr": "cos-smr-magi-00", "char-ngs": "cos-ngs-pair-00"}
	if costume, ok := wantCostumes[info.GetCharacterId()]; !ok || info.GetCostumeId() != costume || info.GetAssetId() == "" || info.GetColor() == "" {
		t.Fatalf("Extra selected an invalid heroine or appearance: %v", info)
	}
}

func assertExtraLoveVisibility(t *testing.T) {
	t.Helper()
	client := papi.NewLoveClient(testConfig.GrpcConn)
	title := loveCall(t, client.TitleExtra, &papi.LoveTitleExtraRequest{LoveId: testExtraLoveID})
	info := title.GetLoveInfo()
	if info.GetLoveId() != testExtraLoveID || info.GetBaseType() != penum.LoveBaseType_LoveBaseType_Extra || !info.GetIsHideManagerVoiceSwitch() || info.GetEndTime() != 0 ||
		!info.GetIsUnlocked() || info.GetIsUnplayed() || title.GetTitleAssetId() != "love-25-0426" || info.GetAssetId() != "love-25-0426" || len(info.GetStoryEndInfos()) != 12 {
		t.Fatalf("TitleExtra lost the permanent Extra event contract: %v", title)
	}
	assertExtraLoveHeroine(t, info.GetMainCharacterInfo())
	top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testExtraLoveID})
	if top.GetExpAssetId() != "exp-love-25-0426" {
		t.Fatal("Extra Top used a standard-series EXP asset namespace")
	}
	heroine := loveCall(t, client.GetMainCharacter, &papi.LoveGetMainCharacterRequest{LoveId: testExtraLoveID})
	assertExtraLoveHeroine(t, heroine.GetMainCharacterInfo())
	user := loveCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	want := moshikoi.VisibleLove(storedLove2024(t, testExtraLoveID))
	if !slices.ContainsFunc(user.GetLoves(), func(love *ptransaction.UserLove) bool { return proto.Equal(love, want) }) ||
		slices.ContainsFunc(user.GetLoves(), func(love *ptransaction.UserLove) bool { return love.GetLoveId() == "love-25-0426" }) {
		t.Fatal("User.Get omitted the Extra event or created an alias progress record under its content ID")
	}
	home := loveCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	if !slices.ContainsFunc(home.GetEventInfo().GetLoveEventInfos(), func(event *papi.HomeLoveEventInfo) bool {
		return event.GetId() == testExtraLoveID && event.GetBaseType() == penum.LoveBaseType_LoveBaseType_Extra && event.GetEndTime() == 0 && event.GetIsUnlocked() && !event.GetIsUnplayed()
	}) {
		t.Fatal("Home.Enter omitted the registered permanent Extra event entrance")
	}
}
