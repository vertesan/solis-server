package test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

const (
	testMeiLoveID = "love-24-0517"
	testYuLoveID  = "love-24-1114"
)

type love2024EndingRoute struct {
	ending, terminal, name string
	prefix                 [][]int32
	choices                []int32
}

func TestLoveMeiAllEndingRoutesScenario(t *testing.T) {
	setupTestUser(t)
	normal := meiHappyPrefix()
	normal[0][0] = 2
	bad := meiHappyPrefix()[:6]
	bad[5][1] = 1
	testLove2024Endings(t, testMeiLoveID, "char-mei", "008-happy-003", []love2024EndingRoute{
		{"003-bad", "003", "ブワッブワッエンド", meiHappyPrefix()[:2], []int32{1, 2, 2}},
		{"008-happy-001", "008-happy", "部活よりも恋愛エンド", meiHappyPrefix(), []int32{1}},
		{"008-happy-002", "008-happy", "駅伝優勝エンド", meiHappyPrefix(), []int32{2}},
		{"008-happy-003", "008-happy", "TRUE エンド", meiHappyPrefix(), []int32{3}},
		{"008-normal-001", "008-normal", "結婚約束エンド", normal, []int32{1}},
		{"008-normal-002", "008-normal", "お友達からよろしくエンド", normal, []int32{2}},
		{"008-bad-001", "007-bad", "謝罪エンド", bad, []int32{1}},
		{"008-bad-002", "007-bad", "未読無視エンド", bad, []int32{2}},
	})
}

func TestLoveYuAllEndingRoutesScenario(t *testing.T) {
	setupTestUser(t)
	normal := yuHappyPrefix()
	normal[2][0] = 2
	ignorePhone := yuHappyPrefix()
	ignorePhone[6] = []int32{1} // Groups 2 and 3 are unreachable, not required.
	tooManyPoints := yuHappyPrefix()
	tooManyPoints[1][0], tooManyPoints[4][0] = 1, 1 // 1+3=4; all happy flags still satisfied.
	testLove2024Endings(t, testYuLoveID, "char-yu", "008-happy-001", []love2024EndingRoute{
		{"003-bad", "004", "スズキスズキスズキEND", yuHappyPrefix()[:3], []int32{1, 2}},
		{"008-happy-001", "008-happy", "TRUEEND", yuHappyPrefix(), []int32{1}},
		{"008-happy-002", "008-happy", "永遠に君の味方END", yuHappyPrefix(), []int32{2}},
		{"008-happy-003", "008-happy", "解釈違いEND", yuHappyPrefix(), []int32{3}},
		{"008-normal-001", "008-normal", "秘密の恋愛END", normal, []int32{1}},
		{"008-normal-002", "008-normal", "かけがえのない友達END", normal, []int32{2}},
		{"008-normal-003", "008-normal", "生涯独身END", normal, []int32{3}},
		{"008-bad-001", "008-bad", "コワイコワイEND", ignorePhone, []int32{1}},
		{"008-bad-002", "008-bad", "泡沫の夢END", tooManyPoints, []int32{2}},
	})
}

func testLove2024Endings(t *testing.T, id, heroine, trueEnding string, cases []love2024EndingRoute) {
	t.Helper()
	client := registerLove2024Series(t)
	catalog := prepareLove2024Series(t, id)
	mainCharacter := loveCall(t, client.GetMainCharacter, &papi.LoveGetMainCharacterRequest{LoveId: id})
	if mainCharacter.GetMainCharacterInfo().GetCharacterId() != heroine {
		t.Fatalf("%s used another series' heroine: %v", id, mainCharacter)
	}
	missionClient := papi.NewMissionClient(testConfig.GrpcConn)
	mission := &papi.MissionReceiveRequest{MissionIds: []string{"event-" + id + "-limited-mission_love-end_emb-sp"}}
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, mission)
	trueClaimed := false
	for index, route := range cases {
		if index > 0 {
			reset := loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: id})
			if !slices.Equal(reset.GetStoryEpisodeIds(), []string{love2024Episode(id, "001")}) {
				t.Fatalf("%s reset restored the wrong first episode", id)
			}
		}
		for episode, choices := range route.prefix {
			playLove2024Episode(t, client, catalog, fmt.Sprintf("%03d", episode+1), len(cases), choices...)
		}
		top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: id})
		if !slices.Contains(top.GetStoryEpisodeIds(), love2024Episode(id, route.terminal)) {
			t.Fatalf("%s route %s reached the wrong terminal: %v", id, route.ending, top.GetStoryEpisodeIds())
		}
		finish := playLove2024Episode(t, client, catalog, route.terminal, len(cases), route.choices...)
		stored := storedLove2024(t, id)
		if !slices.Contains(stored.GetUnlockedLoveStoryEndIDs(), "love-end-"+id+"-"+route.ending) ||
			len(stored.GetUnlockedLoveStoryEndIDs()) != index+1 || finish.GetUnlockSave() != (index == 0) ||
			strings.ReplaceAll(finish.GetStoryEndName(), " ", "") != strings.ReplaceAll(route.name, " ", "") {
			t.Fatalf("%s failed to persist ending %s: finish=%v stored=%v", id, route.ending, finish, stored)
		}
		if route.ending == trueEnding {
			claimed := loveCall(t, missionClient.Receive, mission)
			if !slices.ContainsFunc(claimed.GetResults(), func(reward *ptransaction.RewardResult) bool {
				return reward.GetResourceType() == penum.ResourceType_ResourceType_Emblem && reward.GetResourceId() == "emb-"+id+"-true-sp" && reward.GetAmountI64() == 1
			}) {
				t.Fatalf("%s TRUE ending did not award its own emblem: %v", id, claimed)
			}
			trueClaimed = true
		} else if !trueClaimed {
			// Mei's first two happy endings are not TRUE. This also detects a
			// copied Nagisa/Yu ending-condition mapping pointing at happy-001.
			loveReject(t, codes.FailedPrecondition, missionClient.Receive, mission)
		}
	}
	if !trueClaimed {
		t.Fatal("fixture omitted the TRUE ending")
	}
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, mission)
	if _, err := opuser.FindUserEmblem(context.Background(), testConfig.TestUserSuid, "emb-"+id+"-true-sp"); err != nil {
		t.Fatalf("%s TRUE emblem was not stored: %v", id, err)
	}
	// Completing every ending removes story prices, including formerly paid
	// episodes, and replaying an ending never repeats its permanent bonus.
	setLoveItem(t, catalog.Config.StoryReadItemID, 0)
	before := proto.Clone(storedLove2024(t, id)).(*ptransaction.UserLove)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: id})
	for episode, choices := range cases[0].prefix {
		playLove2024Episode(t, client, catalog, fmt.Sprintf("%03d", episode+1), len(cases), choices...)
	}
	playLove2024Episode(t, client, catalog, cases[0].terminal, len(cases), cases[0].choices...)
	if !proto.Equal(before, storedLove2024(t, id)) || loveItemAmount(t, catalog.Config.StoryReadItemID) != 0 {
		t.Fatalf("%s all-ending replay charged story points or repeated its ending bonus", id)
	}
	assertLove2024Visibility(t)
}

func TestLoveMeiNormalFlagBranchesScenario(t *testing.T) {
	flags := [][2]int{{1, 1}, {1, 2}, {2, 1}, {2, 2}, {3, 1}, {3, 2}, {4, 1}, {5, 1}, {5, 2}, {5, 3}, {6, 1}, {7, 1}, {7, 2}}
	for _, flag := range append([][2]int{{0, 0}}, flags...) {
		t.Run(fmt.Sprintf("episode%d_group%d", flag[0], flag[1]), func(t *testing.T) {
			path, want := meiHappyPrefix(), "008-happy"
			if flag[0] > 0 {
				path[flag[0]-1][flag[1]-1] = 3 - path[flag[0]-1][flag[1]-1]
				want = "008-normal"
			}
			if got := resolveLove2024Prefix(t, testMeiLoveID, path); got != love2024Episode(testMeiLoveID, want) {
				t.Fatalf("one normal flag routed to %q, want %s", got, want)
			}
		})
	}
}

func TestLoveYuPointAndFlagBranchesScenario(t *testing.T) {
	// Expectations use the handwritten map's numeric counter, independently
	// of the route YAML's boolean conditions: four awards weigh 1,1,3,2.
	for mask := range 16 {
		for _, missingHappyFlag := range []bool{false, true} {
			t.Run(fmt.Sprintf("points_mask_%04b_missing_flag_%t", mask, missingHappyFlag), func(t *testing.T) {
				path, points := yuHappyPrefix(), 0
				for bit, award := range []struct{ episode, group, choice, points int }{{2, 1, 1, 1}, {2, 2, 2, 1}, {5, 1, 1, 3}, {7, 3, 2, 2}} {
					if mask&(1<<bit) != 0 {
						path[award.episode-1][award.group-1] = int32(award.choice)
						points += award.points
					}
				}
				if missingHappyFlag {
					path[2][0] = 2
				}
				want := "008-happy"
				if points >= 4 {
					want = "008-bad"
				} else if missingHappyFlag || mask&8 != 0 {
					want = "008-normal"
				}
				if got := resolveLove2024Prefix(t, testYuLoveID, path); got != love2024Episode(testYuLoveID, want) {
					t.Fatalf("%d points routed to %q, want %s", points, got, want)
				}
			})
		}
	}
	// Missing any non-terminating happy flag independently selects normal.
	for _, flag := range [][2]int{{3, 1}, {3, 2}, {4, 1}, {5, 2}, {6, 1}, {6, 2}, {7, 3}} {
		path := yuHappyPrefix()
		path[flag[0]-1][flag[1]-1] = 2
		if got := resolveLove2024Prefix(t, testYuLoveID, path); got != love2024Episode(testYuLoveID, "008-normal") {
			t.Fatalf("missing happy flag at %v reached %s", flag, got)
		}
	}
	ignoredPhone := yuHappyPrefix()
	ignoredPhone[6] = []int32{1}
	if got := resolveLove2024Prefix(t, testYuLoveID, ignoredPhone); got != love2024Episode(testYuLoveID, "008-bad") {
		t.Fatalf("ignoring the phone with zero points did not take priority: %s", got)
	}
	routes, err := moshikoi.LoadRoutes(testYuLoveID)
	if err != nil {
		t.Fatal(err)
	}
	id := love2024Episode(testYuLoveID, "007")
	if _, err := routes.SelectChoices(id, love2024FinishRequest(id, 1, 1, 1).GetSelectStoryChoiceInfos()); err == nil {
		t.Fatal("unreachable phone-answer choice groups were accepted as active selections")
	}
	echoed := love2024FinishRequest(id, 1, 1, 2)
	echoed.SelectStoryChoiceInfos[1].IsValid, echoed.SelectStoryChoiceInfos[2].IsValid = false, false
	selected, err := routes.SelectChoices(id, echoed.GetSelectStoryChoiceInfos())
	if err != nil || len(selected) != 1 {
		t.Fatalf("historical unreachable choices were not ignored: %v", err)
	}
}

func TestLove2024SaveLoadIsolationScenario(t *testing.T) {
	setupTestUser(t)
	client := registerLove2024Series(t)
	mei, yu := prepareLove2024Series(t, testMeiLoveID), prepareLove2024Series(t, testYuLoveID)
	// Unlock saving through each event's own early ending first.
	for i, choices := range meiHappyPrefix()[:2] {
		playLove2024Episode(t, client, mei, fmt.Sprintf("%03d", i+1), 8, choices...)
	}
	playLove2024Episode(t, client, mei, "003", 8, 1, 2, 2)
	for i, choices := range yuHappyPrefix()[:3] {
		playLove2024Episode(t, client, yu, fmt.Sprintf("%03d", i+1), 9, choices...)
	}
	playLove2024Episode(t, client, yu, "004", 9, 1, 2)
	playLove2024Episode(t, client, mei, "001", 8, 2, 1)
	loveCall(t, client.Save, &papi.LoveSaveRequest{LoveId: testMeiLoveID, SlotNumber: 1})
	loveCall(t, client.UpdateSaveName, &papi.LoveUpdateSaveNameRequest{LoveId: testMeiLoveID, SlotNumber: 1, Name: "Mei normal"})
	meiBefore := love2024State(t, testMeiLoveID)
	meiLoveBefore := proto.Clone(storedLove2024(t, testMeiLoveID)).(*ptransaction.UserLove)

	// Store Yu's 1+3=4 point route; later make a zero-point route, then load.
	pointsPath := yuHappyPrefix()
	pointsPath[1][0], pointsPath[4][0] = 1, 1
	for i, choices := range pointsPath[:5] {
		playLove2024Episode(t, client, yu, fmt.Sprintf("%03d", i+1), 9, choices...)
	}
	loveCall(t, client.Save, &papi.LoveSaveRequest{LoveId: testYuLoveID, SlotNumber: 1})
	loveCall(t, client.UpdateSaveName, &papi.LoveUpdateSaveNameRequest{LoveId: testYuLoveID, SlotNumber: 1, Name: "Yu four"})
	savedLove := proto.Clone(storedLove2024(t, testYuLoveID)).(*ptransaction.UserLove)
	savedState := love2024State(t, testYuLoveID)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testYuLoveID})
	for i, choices := range yuHappyPrefix()[:5] {
		playLove2024Episode(t, client, yu, fmt.Sprintf("%03d", i+1), 9, choices...)
	}
	setLoveItem(t, yu.Config.PresentItemID, 1)
	loveCall(t, client.Present, &papi.LovePresentRequest{LoveId: testYuLoveID})
	pointsBeforeLoad := loveItemAmount(t, yu.Config.StoryReadItemID)
	loveCall(t, client.Load, &papi.LoveLoadRequest{LoveId: testYuLoveID, SlotNumber: 1})
	loadedState, loadedLove := love2024State(t, testYuLoveID), storedLove2024(t, testYuLoveID)
	if !slices.Equal(loadedState.Selections, savedState.Selections) || !slices.Equal(loadedState.StoryEpisodeIDs, savedState.StoryEpisodeIDs) ||
		loadedLove.GetHiddenFavorability() != savedLove.GetHiddenFavorability()+yu.Config.PresentFavorability ||
		!slices.Equal(loadedLove.GetUnlockedLoveStoryEndIDs(), savedLove.GetUnlockedLoveStoryEndIDs()) ||
		loadedState.Exp != savedState.Exp || loveItemAmount(t, yu.Config.StoryReadItemID) != pointsBeforeLoad || loveItemAmount(t, yu.Config.PresentItemID) != 0 {
		t.Fatal("loading Yu's route lost its choices or rolled back permanent progress/resources")
	}
	playLove2024Episode(t, client, yu, "006", 9, 1, 1)
	playLove2024Episode(t, client, yu, "007", 9, 2, 1, 1)
	if !slices.Contains(love2024State(t, testYuLoveID).StoryEpisodeIDs, love2024Episode(testYuLoveID, "008-bad")) {
		t.Fatal("loaded Yu choices did not reconstruct the four-point bad-route threshold")
	}
	meiAfter := love2024State(t, testMeiLoveID)
	if !slices.Equal(meiBefore.Selections, meiAfter.Selections) || !slices.Equal(meiBefore.StoryEpisodeIDs, meiAfter.StoryEpisodeIDs) ||
		!proto.Equal(meiBefore.Saves[0].Info, meiAfter.Saves[0].Info) || !slices.Equal(meiBefore.Saves[0].Selections, meiAfter.Saves[0].Selections) ||
		!proto.Equal(meiLoveBefore, storedLove2024(t, testMeiLoveID)) {
		t.Fatal("Yu's slot 1 or progression overwrote Mei's slot 1 or progression")
	}
	yuBeforeMei := love2024State(t, testYuLoveID)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testMeiLoveID})
	playLove2024Episode(t, client, mei, "001", 8, 1, 1)
	loveCall(t, client.Load, &papi.LoveLoadRequest{LoveId: testMeiLoveID, SlotNumber: 1})
	for index, choices := range meiHappyPrefix()[1:] {
		playLove2024Episode(t, client, mei, fmt.Sprintf("%03d", index+2), 8, choices...)
	}
	if !slices.Contains(love2024State(t, testMeiLoveID).StoryEpisodeIDs, love2024Episode(testMeiLoveID, "008-normal")) {
		t.Fatal("loading Mei's route lost its early normal flag")
	}
	yuAfterMei := love2024State(t, testYuLoveID)
	if !slices.Equal(yuBeforeMei.Selections, yuAfterMei.Selections) || !slices.Equal(yuBeforeMei.StoryEpisodeIDs, yuAfterMei.StoryEpisodeIDs) ||
		!proto.Equal(yuBeforeMei.Saves[0].Info, yuAfterMei.Saves[0].Info) || !slices.Equal(yuBeforeMei.Saves[0].Selections, yuAfterMei.Saves[0].Selections) {
		t.Fatal("loading Mei affected Yu's stored route or save slot")
	}
	assertLove2024Visibility(t)
}

func meiHappyPrefix() [][]int32 {
	return [][]int32{{1, 1}, {2, 1}, {1, 2, 1}, {2}, {1, 1, 2}, {1, 2}, {2, 1}}
}

func yuHappyPrefix() [][]int32 {
	return [][]int32{{1, 1}, {2, 1}, {1, 1}, {1, 1}, {2, 1}, {1, 1}, {2, 1, 1}}
}

func love2024Episode(id, suffix string) string { return "love-event-story-" + id + "-" + suffix }

func love2024FinishRequest(episodeID string, choices ...int32) *papi.LoveFinishRequest {
	request := &papi.LoveFinishRequest{StoryEpisodeId: episodeID}
	for i, choice := range choices {
		request.SelectStoryChoiceInfos = append(request.SelectStoryChoiceInfos, &papi.LoveSelectStoryChoiceInfo{GroupIndex: int32(i + 1), Index: choice, IsValid: true})
	}
	return request
}

func registerLove2024Series(t *testing.T) papi.LoveClient {
	t.Helper()
	unlockTestLove(t)
	client := papi.NewLoveClient(testConfig.GrpcConn)
	for _, id := range []string{testLoveID, testNagisaLoveID, testMeiLoveID, testYuLoveID} {
		loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: id, Name: "Manager"})
	}
	return client
}

func prepareLove2024Series(t *testing.T, id string) *moshikoi.Catalog {
	t.Helper()
	catalog, err := moshikoi.LoadCatalog(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	state := love2024State(t, id)
	state.Exp = catalog.Levels[len(catalog.Levels)-1].GetRequiredExp()
	storeLoveState(t, state)
	setLoveItem(t, catalog.Config.StoryReadItemID, 1_000_000)
	return catalog
}

func love2024State(t *testing.T, id string) *umodel.UserLoveState {
	t.Helper()
	state, err := opuser.FindUserLoveState(context.Background(), testConfig.TestUserSuid, id)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func storedLove2024(t *testing.T, id string) *ptransaction.UserLove {
	t.Helper()
	love, err := moshikoi.UserLove(context.Background(), testConfig.TestUserSuid, id)
	if err != nil {
		t.Fatal(err)
	}
	return love
}

func playLove2024Episode(t *testing.T, client papi.LoveClient, catalog *moshikoi.Catalog, suffix string, endingCount int, choices ...int32) *papi.LoveFinishResponse {
	t.Helper()
	id := love2024Episode(catalog.LoveID, suffix)
	master := catalog.Episode(id)
	if master == nil {
		t.Fatalf("missing master episode %s", id)
	}
	before, cost := loveItemAmount(t, catalog.Config.StoryReadItemID), int64(master.GetConsumptionItemAmount())
	if len(storedLove2024(t, catalog.LoveID).GetUnlockedLoveStoryEndIDs()) == endingCount {
		cost = 0
	}
	start := loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: id})
	if catalog.LoveID == testYuLoveID && suffix == "008-bad" {
		wantBranch := int32(2)
		if slices.Contains(love2024State(t, testYuLoveID).Selections, umodel.LoveStorySelection{
			StoryEpisodeID: love2024Episode(testYuLoveID, "007"), GroupIndex: 1, Index: 1,
		}) {
			wantBranch = 1
		}
		branches := start.GetAdvBranchInfos()
		if len(branches) != 1 || branches[0].GetGroupIndex() != 1 || branches[0].GetIndex() != wantBranch {
			t.Fatalf("Yu bad-ending intro chose the wrong phone/point script branch: want=%d got=%v", wantBranch, branches)
		}
	}
	if loveItemAmount(t, catalog.Config.StoryReadItemID) != before-cost {
		t.Fatalf("%s did not debit its expected master story-point cost %d", id, cost)
	}
	finish := loveCall(t, client.Finish, love2024FinishRequest(id, choices...))
	want := storedLove2024(t, catalog.LoveID)
	if !slices.ContainsFunc(finish.GetCommonResponse().GetUpdatedData().GetLoves(), func(row *ptransaction.UserLove) bool {
		return proto.Equal(row, moshikoi.VisibleLove(want)) && !row.GetIsReadingStory()
	}) {
		t.Fatalf("%s Finish omitted the persisted Love post-image", id)
	}
	return finish
}

func resolveLove2024Prefix(t *testing.T, id string, path [][]int32) string {
	t.Helper()
	routes, err := moshikoi.LoadRoutes(id)
	if err != nil {
		t.Fatal(err)
	}
	var selections []umodel.LoveStorySelection
	var next string
	for index, choices := range path {
		episode := love2024Episode(id, fmt.Sprintf("%03d", index+1))
		selected, err := routes.SelectChoices(episode, love2024FinishRequest(episode, choices...).GetSelectStoryChoiceInfos())
		if err != nil {
			t.Fatalf("selecting %s: %v", episode, err)
		}
		selections = append(selections, selected...)
		var ending string
		next, ending, err = routes.Resolve(episode, selections)
		if err != nil || ending != "" {
			t.Fatalf("resolving %s unexpectedly ended the route: ending=%s err=%v", episode, ending, err)
		}
		if index < len(path)-1 && next != love2024Episode(id, fmt.Sprintf("%03d", index+2)) {
			t.Fatalf("resolving %s skipped an intermediate chapter: next=%s", episode, next)
		}
	}
	return next
}

func assertLove2024Visibility(t *testing.T) {
	t.Helper()
	user := loveCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	home := loveCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	assertHomeLoveEntrances(t, home)
	for _, id := range []string{testLoveID, testNagisaLoveID, testMeiLoveID, testYuLoveID} {
		want := moshikoi.VisibleLove(storedLove2024(t, id))
		if !slices.ContainsFunc(user.GetLoves(), func(row *ptransaction.UserLove) bool { return proto.Equal(row, want) }) {
			t.Fatalf("User.Get omitted persisted series %s", id)
		}
	}
}
