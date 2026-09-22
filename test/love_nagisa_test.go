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

const testNagisaLoveID = "love-23-1114"

func TestLoveNagisaAllEndingRoutesScenario(t *testing.T) {
	setupTestUser(t)
	client, _ := registerTestLove(t)
	reiBefore := proto.Clone(storedLove(t)).(*ptransaction.UserLove)
	reiStateBefore := loveState(t)
	catalog, err := moshikoi.LoadCatalog(context.Background(), testNagisaLoveID)
	if err != nil {
		t.Fatal(err)
	}
	title := loveCall(t, client.Title, &papi.Empty{})
	if !slices.ContainsFunc(title.GetLoveInfos(), func(info *papi.LoveInfo) bool {
		return info.GetLoveId() == testNagisaLoveID && info.GetEndTime() == 0 && info.GetIsUnplayed() && info.GetIsUnlocked()
	}) {
		t.Fatalf("Title omitted the unlocked permanent Nagisa event: %v", title)
	}
	loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: testNagisaLoveID, Name: "Nagisa"})
	state, err := opuser.FindUserLoveState(context.Background(), testConfig.TestUserSuid, testNagisaLoveID)
	if err != nil {
		t.Fatal(err)
	}
	state.Exp = catalog.Levels[len(catalog.Levels)-1].GetRequiredExp()
	storeLoveState(t, state)
	setLoveItem(t, catalog.Config.StoryReadItemID, 1_000_000)
	mainCharacter := loveCall(t, client.GetMainCharacter, &papi.LoveGetMainCharacterRequest{LoveId: testNagisaLoveID})
	if mainCharacter.GetMainCharacterInfo().GetCharacterId() != "char-ngs" {
		t.Fatalf("Nagisa event used another series' heroine: %v", mainCharacter)
	}
	missionClient := papi.NewMissionClient(testConfig.GrpcConn)
	trueMission := &papi.MissionReceiveRequest{MissionIds: []string{"event-love-23-1114-limited-mission_love-end_emb-sp"}}
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, trueMission)

	// The handwritten route map numbers alternatives from zero; the protocol
	// uses one-based indices. Expectations below are independent of route YAML.
	routes := []struct {
		ending, terminal, name string
		choice                 int32
		normal, bad            bool
	}{
		{ending: "002-bad", terminal: "002", name: "スズキスズキエンド"},
		{ending: "008-happy-001", terminal: "008-happy", name: "TRUE エンド", choice: 1},
		{ending: "008-happy-002", terminal: "008-happy", name: "充実した日々エンド", choice: 2},
		{ending: "008-happy-003", terminal: "008-happy", name: "小説の糧エンド", choice: 3},
		{ending: "008-normal-001", terminal: "008-normal", name: "現実は甘くないエンド", choice: 1, normal: true},
		{ending: "008-normal-002", terminal: "008-normal", name: "互いの道エンド", choice: 2, normal: true},
		{ending: "008-bad-001", terminal: "007-bad", name: "今更エンド", choice: 1, bad: true},
		{ending: "008-bad-002", terminal: "007-bad", name: "見つめなおしエンド", choice: 2, bad: true},
	}
	for i, route := range routes {
		t.Run(route.ending, func(t *testing.T) {
			if i > 0 {
				reset := loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testNagisaLoveID})
				if !slices.Equal(reset.GetStoryEpisodeIds(), []string{nagisaEpisode("001")}) {
					t.Fatalf("reset did not restore Nagisa's opening: %v", reset)
				}
			}
			path := nagisaHappyPrefix()
			if route.normal {
				path[0][1] = 2 // One normal flag is enough, despite all later happy choices.
			}
			if route.bad {
				path[5][1] = 2
			}
			prefixLength := 7
			if route.bad {
				prefixLength = 6
			} else if route.terminal == "002" {
				prefixLength = 1
			}
			for episode := range prefixLength {
				playNagisaEpisode(t, client, catalog, fmt.Sprintf("%03d", episode+1), path[episode]...)
			}
			top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testNagisaLoveID})
			if !slices.Contains(top.GetStoryEpisodeIds(), nagisaEpisode(route.terminal)) {
				t.Fatalf("wrong terminal episode for %s: %v", route.ending, top.GetStoryEpisodeIds())
			}
			terminalChoices := []int32{route.choice}
			if route.terminal == "002" {
				terminalChoices = []int32{1, 2}
			}
			finish := playNagisaEpisode(t, client, catalog, route.terminal, terminalChoices...)
			love := storedNagisaLove(t)
			if !slices.Contains(love.GetUnlockedLoveStoryEndIDs(), "love-end-"+testNagisaLoveID+"-"+route.ending) ||
				len(love.GetUnlockedLoveStoryEndIDs()) != i+1 || finish.GetUnlockSave() != (i == 0) ||
				strings.ReplaceAll(finish.GetStoryEndName(), " ", "") != strings.ReplaceAll(route.name, " ", "") {
				t.Fatalf("unexpected ending %s: finish=%v persisted=%v", route.ending, finish, love)
			}
			if i == 0 {
				loveReject(t, codes.FailedPrecondition, missionClient.Receive, trueMission)
			} else if i == 1 {
				claim := loveCall(t, missionClient.Receive, trueMission)
				if !slices.ContainsFunc(claim.GetResults(), func(result *ptransaction.RewardResult) bool {
					return result.GetResourceType() == penum.ResourceType_ResourceType_Emblem &&
						result.GetResourceId() == "emb-love-23-1114-true-sp" && result.GetAmountI64() == 1
				}) {
					t.Fatalf("TRUE ending did not grant the Nagisa emblem: %v", claim)
				}
			}
		})
	}
	// Once all endings are recorded, both free and formerly paid episodes must
	// replay for zero story points and must not grant an ending bonus twice.
	setLoveItem(t, catalog.Config.StoryReadItemID, 0)
	beforeRepeat := proto.Clone(storedNagisaLove(t)).(*ptransaction.UserLove)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testNagisaLoveID})
	playNagisaEpisode(t, client, catalog, "001", 1, 1)
	playNagisaEpisode(t, client, catalog, "002", 1, 2)
	if !proto.Equal(beforeRepeat, storedNagisaLove(t)) || loveItemAmount(t, catalog.Config.StoryReadItemID) != 0 {
		t.Fatal("all-ending replay charged points or repeated permanent ending rewards")
	}
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, trueMission)
	if _, err := opuser.FindUserEmblem(context.Background(), testConfig.TestUserSuid, "emb-love-23-1114-true-sp"); err != nil {
		t.Fatalf("claimed Nagisa emblem was not persisted: %v", err)
	}
	if !proto.Equal(reiBefore, storedLove(t)) {
		t.Fatal("playing Nagisa changed Rei's permanent progress")
	}
	reiStateAfter := loveState(t)
	if reiStateAfter.Exp != reiStateBefore.Exp || !slices.Equal(reiStateAfter.Selections, reiStateBefore.Selections) ||
		!slices.Equal(reiStateAfter.StoryEpisodeIDs, reiStateBefore.StoryEpisodeIDs) || reiStateAfter.PlayerName != reiStateBefore.PlayerName {
		t.Fatal("playing Nagisa changed Rei's current route, EXP or name")
	}
	assertNagisaVisibility(t)
}

func TestLoveNagisaNormalFlagBranchesScenario(t *testing.T) {
	routes, err := moshikoi.LoadRoutes(testNagisaLoveID)
	if err != nil {
		t.Fatal(err)
	}
	// Each unfavorable choice independently sets the normal flag. Testing one
	// at a time catches an accidental AND or a flag overwritten by later choices.
	flags := [][2]int{{1, 2}, {2, 1}, {3, 1}, {3, 2}, {4, 1}, {4, 2}, {5, 1}, {5, 2}, {5, 3}, {7, 1}, {7, 2}}
	for _, flag := range append([][2]int{{0, 0}}, flags...) {
		t.Run(fmt.Sprintf("episode%d_choice%d", flag[0], flag[1]), func(t *testing.T) {
			path := nagisaHappyPrefix()
			wantTerminal := nagisaEpisode("008-happy")
			if flag[0] > 0 {
				path[flag[0]-1][flag[1]-1] = 3 - path[flag[0]-1][flag[1]-1]
				wantTerminal = nagisaEpisode("008-normal")
			}
			var selections []umodel.LoveStorySelection
			for index, choices := range path {
				episodeID := nagisaEpisode(fmt.Sprintf("%03d", index+1))
				selected, err := routes.SelectChoices(episodeID, nagisaFinishRequest(episodeID, choices...).GetSelectStoryChoiceInfos())
				if err != nil {
					t.Fatal(err)
				}
				selections = append(selections, selected...)
				next, ending, err := routes.Resolve(episodeID, selections)
				wantNext := nagisaEpisode(fmt.Sprintf("%03d", index+2))
				if index == len(path)-1 {
					wantNext = wantTerminal
				}
				if err != nil || ending != "" || next != wantNext {
					t.Fatalf("wrong branch after %s: next=%q ending=%q err=%v want=%q", episodeID, next, ending, err, wantNext)
				}
			}
		})
	}
}

func TestLoveNagisaSaveLoadIsolationScenario(t *testing.T) {
	setupTestUser(t)
	client, _ := registerTestLove(t)
	raiseTestLoveLevel(t, loveCatalog(t))
	lovePlayEpisode(t, client, "001", 2, 2)
	loveCall(t, client.Save, &papi.LoveSaveRequest{LoveId: testLoveID, SlotNumber: 1})
	loveCall(t, client.UpdateSaveName, &papi.LoveUpdateSaveNameRequest{LoveId: testLoveID, SlotNumber: 1, Name: "Rei save"})
	reiBefore := loveState(t)
	loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: testNagisaLoveID, Name: "Manager"})
	catalog, err := moshikoi.LoadCatalog(context.Background(), testNagisaLoveID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := opuser.FindUserLoveState(context.Background(), testConfig.TestUserSuid, testNagisaLoveID)
	if err != nil {
		t.Fatal(err)
	}
	state.Exp = catalog.Levels[len(catalog.Levels)-1].GetRequiredExp()
	storeLoveState(t, state)
	setLoveItem(t, catalog.Config.StoryReadItemID, 1_000_000)
	playNagisaEpisode(t, client, catalog, "001", 1, 1)
	playNagisaEpisode(t, client, catalog, "002", 1, 2)
	// Save a normal-flag route, then change the route before loading the save.
	playNagisaEpisode(t, client, catalog, "001", 1, 2)
	loveCall(t, client.Save, &papi.LoveSaveRequest{LoveId: testNagisaLoveID, SlotNumber: 1})
	loveCall(t, client.UpdateSaveName, &papi.LoveUpdateSaveNameRequest{LoveId: testNagisaLoveID, SlotNumber: 1, Name: "Nagisa save"})
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testNagisaLoveID})
	playNagisaEpisode(t, client, catalog, "001", 1, 1)
	pointsBeforeLoad := loveItemAmount(t, catalog.Config.StoryReadItemID)
	load := loveCall(t, client.Load, &papi.LoveLoadRequest{LoveId: testNagisaLoveID, SlotNumber: 1})
	if !slices.Equal(load.GetStoryEpisodeIds(), []string{nagisaEpisode("001"), nagisaEpisode("002")}) ||
		loveItemAmount(t, catalog.Config.StoryReadItemID) != pointsBeforeLoad {
		t.Fatalf("Nagisa save did not restore only the route: %v", load)
	}
	for index, choices := range nagisaHappyPrefix()[1:] {
		playNagisaEpisode(t, client, catalog, fmt.Sprintf("%03d", index+2), choices...)
	}
	top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testNagisaLoveID})
	if !slices.Contains(top.GetStoryEpisodeIds(), nagisaEpisode("008-normal")) ||
		slices.Contains(top.GetStoryEpisodeIds(), nagisaEpisode("008-happy")) {
		t.Fatalf("loading forgot the saved early normal flag: %v", top.GetStoryEpisodeIds())
	}
	reiAfter := loveState(t)
	if len(reiBefore.Saves) != 1 || len(reiAfter.Saves) != 1 || !proto.Equal(reiBefore.Saves[0].Info, reiAfter.Saves[0].Info) ||
		!slices.Equal(reiBefore.Saves[0].StoryEpisodeIDs, reiAfter.Saves[0].StoryEpisodeIDs) ||
		!slices.Equal(reiBefore.Saves[0].Selections, reiAfter.Saves[0].Selections) {
		t.Fatal("Nagisa save slot 1 overwrote Rei save slot 1")
	}
}

func nagisaHappyPrefix() [][]int32 {
	return [][]int32{{1, 1}, {1, 1}, {2, 1}, {1, 2}, {2, 1, 2}, {1, 1}, {2, 1}}
}

func nagisaEpisode(suffix string) string {
	return "love-event-story-" + testNagisaLoveID + "-" + suffix
}

func nagisaFinishRequest(episodeID string, choices ...int32) *papi.LoveFinishRequest {
	request := &papi.LoveFinishRequest{StoryEpisodeId: episodeID}
	for i, choice := range choices {
		request.SelectStoryChoiceInfos = append(request.SelectStoryChoiceInfos, &papi.LoveSelectStoryChoiceInfo{
			GroupIndex: int32(i + 1), Index: choice, IsValid: true,
		})
	}
	return request
}

func playNagisaEpisode(t *testing.T, client papi.LoveClient, catalog *moshikoi.Catalog, suffix string, choices ...int32) *papi.LoveFinishResponse {
	t.Helper()
	id := nagisaEpisode(suffix)
	before := loveItemAmount(t, catalog.Config.StoryReadItemID)
	cost := int64(catalog.Episode(id).GetConsumptionItemAmount())
	if len(storedNagisaLove(t).GetUnlockedLoveStoryEndIDs()) == 8 {
		cost = 0
	}
	loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: id})
	if loveItemAmount(t, catalog.Config.StoryReadItemID) != before-cost {
		t.Fatalf("%s did not charge the master-defined story price %d", id, cost)
	}
	finish := loveCall(t, client.Finish, nagisaFinishRequest(id, choices...))
	want := storedNagisaLove(t)
	if !slices.ContainsFunc(finish.GetCommonResponse().GetUpdatedData().GetLoves(), func(row *ptransaction.UserLove) bool {
		return proto.Equal(row, moshikoi.VisibleLove(want)) && !row.GetIsReadingStory()
	}) {
		t.Fatalf("Finish omitted Nagisa's persisted post-image: %v", finish)
	}
	return finish
}

func storedNagisaLove(t *testing.T) *ptransaction.UserLove {
	t.Helper()
	love, err := moshikoi.UserLove(context.Background(), testConfig.TestUserSuid, testNagisaLoveID)
	if err != nil {
		t.Fatal(err)
	}
	return love
}

func assertNagisaVisibility(t *testing.T) {
	t.Helper()
	user := loveCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	for _, expected := range []*ptransaction.UserLove{storedLove(t), storedNagisaLove(t)} {
		if !slices.ContainsFunc(user.GetLoves(), func(row *ptransaction.UserLove) bool { return proto.Equal(row, moshikoi.VisibleLove(expected)) }) {
			t.Fatalf("User.Get omitted or changed persisted series %s", expected.GetLoveId())
		}
	}
	home := loveCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	for _, id := range []string{testLoveID, testNagisaLoveID} {
		found := false
		for _, event := range home.GetEventInfo().GetLoveEventInfos() {
			if !strings.Contains(event.GetId(), id) {
				continue
			}
			if event.GetEndTime() != 0 || !event.GetIsUnlocked() || event.GetIsUnplayed() {
				t.Fatalf("Home.Enter lost the permanent registered event %s: %v", id, event)
			}
			found = true
		}
		if !found {
			t.Fatalf("Home.Enter omitted event %s", id)
		}
	}
}
