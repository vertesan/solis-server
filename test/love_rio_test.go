package test

import (
	"context"
	"fmt"
	"slices"
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

const testRioLoveID = "love-26-0313"

func TestLoveRioAllEndingRoutesScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerRioLove(t)
	mainCharacter := loveCall(t, client.GetMainCharacter, &papi.LoveGetMainCharacterRequest{LoveId: testRioLoveID})
	if mainCharacter.GetMainCharacterInfo().GetCharacterId() != "char-rio" {
		t.Fatal("Rio event selected the wrong heroine")
	}
	normal := rioHappyPrefix()
	normal[0][0] = 2 // Exactly two negative points enter the normal branch.
	cases := []love2024EndingRoute{
		{"001-bad", "001", "コドクボッチEND", nil, []int32{1, 1, 2}},
		{"008-happy-002", "008-happy", "君の専属マネージャーEND", rioHappyPrefix(), []int32{2}},
		{"008-happy-001", "008-happy", "TRUEEND", rioHappyPrefix(), []int32{1, 1}},
		{"008-normal-001", "008-normal", "半額弁当END", normal, []int32{1}},
		{"008-normal-002", "008-normal", "もしも瑠依と恋をしたらEND", normal, []int32{2, 1}},
		{"008-normal-003", "008-normal", "時を刻む恋慕END", normal, []int32{2, 2}},
		{"008-bad-001", "008-bad", "コネコネEND", rioMaximumPointPrefix(), []int32{1}},
		{"008-bad-002", "008-bad", "借金END", rioMaximumPointPrefix(), []int32{2, 1}},
		{"008-bad-003", "008-bad", "瑠依のヒモEND", rioMaximumPointPrefix(), []int32{2, 2}},
	}
	missionClient := papi.NewMissionClient(testConfig.GrpcConn)
	mission := &papi.MissionReceiveRequest{MissionIds: []string{"event-love-26-0313-limited-mission_love-end_emb-sp"}}
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, mission)
	for index, route := range cases {
		if index > 0 {
			loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testRioLoveID})
		}
		playRioPrefix(t, client, catalog, route.prefix)
		top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testRioLoveID})
		if !slices.Contains(top.GetStoryEpisodeIds(), love2024Episode(testRioLoveID, route.terminal)) {
			t.Fatalf("Rio %s reached the wrong terminal: %v", route.ending, top.GetStoryEpisodeIds())
		}
		finish := playLove2024Episode(t, client, catalog, route.terminal, 9, route.choices...)
		stored := storedLove2024(t, testRioLoveID)
		if !slices.Contains(stored.GetUnlockedLoveStoryEndIDs(), "love-end-"+testRioLoveID+"-"+route.ending) ||
			len(stored.GetUnlockedLoveStoryEndIDs()) != index+1 || finish.GetUnlockSave() != (index == 0) || finish.GetStoryEndName() != route.name {
			t.Fatalf("Rio ending %s was not persisted correctly: finish=%v stored=%v", route.ending, finish, stored)
		}
		if index < 2 {
			loveReject(t, codes.FailedPrecondition, missionClient.Receive, mission)
		} else if index == 2 {
			claim := loveCall(t, missionClient.Receive, mission)
			if !slices.ContainsFunc(claim.GetResults(), func(reward *ptransaction.RewardResult) bool {
				return reward.GetResourceType() == penum.ResourceType_ResourceType_Emblem && reward.GetResourceId() == "emb-love-26-0313-true-sp" && reward.GetAmountI64() == 1
			}) {
				t.Fatalf("Rio TRUE ending did not grant its own emblem: %v", claim)
			}
		}
	}
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, mission)
	if _, err := opuser.FindUserEmblem(context.Background(), testConfig.TestUserSuid, "emb-love-26-0313-true-sp"); err != nil {
		t.Fatalf("Rio TRUE emblem was not persisted: %v", err)
	}
	// Replay the complete paid route, choosing the other TRUE-ending answer.
	// Both answers give one ending, and all nine endings make replay free.
	setLoveItem(t, catalog.Config.StoryReadItemID, 0)
	before := proto.Clone(storedLove2024(t, testRioLoveID)).(*ptransaction.UserLove)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testRioLoveID})
	playRioPrefix(t, client, catalog, rioHappyPrefix())
	finish := playLove2024Episode(t, client, catalog, "008-happy", 9, 1, 2)
	if finish.GetStoryEndName() != "TRUEEND" || finish.GetUnlockSave() || !proto.Equal(before, storedLove2024(t, testRioLoveID)) || loveItemAmount(t, catalog.Config.StoryReadItemID) != 0 {
		t.Fatal("Rio all-ending replay charged points, duplicated an ending, or changed permanent favorability")
	}
	assertRioLoveVisibility(t)
}

func TestLoveRioPointBoundariesScenario(t *testing.T) {
	routes, err := moshikoi.LoadRoutes(testRioLoveID)
	if err != nil {
		t.Fatal(err)
	}
	// Independent expectations from the user's map, not the YAML. These ten
	// binary awards total 17. The three possible EP6 consultation paths add
	// either 0 or 1, never 2, because group4 is conditional on group3.
	awards := []struct{ episode, group, choice, points int }{
		{1, 1, 2, 2}, {2, 1, 2, 1}, {2, 2, 2, 1}, {3, 1, 1, 2}, {3, 2, 1, 1},
		{4, 1, 1, 1}, {5, 1, 1, 3}, {6, 2, 2, 1}, {7, 1, 2, 2}, {7, 2, 2, 3},
	}
	seenTotals := map[int]bool{}
	for mask := range 1 << len(awards) {
		for consultation := range 3 {
			path, points := rioHappyPrefix(), 0
			for bit, award := range awards {
				if mask&(1<<bit) != 0 {
					path[award.episode-1][award.group-1] = int32(award.choice)
					points += award.points
				}
			}
			switch consultation {
			case 0: // No consultation: group4 must be absent, +1.
				path[5] = path[5][:3]
				path[5][2] = 1
				points++
			case 1: // Consult, then visit restaurant: group4 option1, +1.
				path[5][3] = 1
				points++
			}
			// Vary neutral answers too, including the three-option meal question.
			path[0][1], path[1][2], path[3][1] = int32(mask%2+1), int32(mask%2+1), int32(mask%2+1)
			path[4][1], path[4][2], path[5][0] = int32(mask%2+1), int32(mask%2+1), int32(mask%3+1)
			want := "008-normal"
			if points <= 1 {
				want = "008-happy"
			} else if points >= 11 {
				want = "008-bad"
			}
			var selections []umodel.LoveStorySelection
			for index, choices := range path {
				id := love2024Episode(testRioLoveID, fmt.Sprintf("%03d", index+1))
				selected, err := routes.SelectChoices(id, love2024FinishRequest(id, choices...).GetSelectStoryChoiceInfos())
				if err != nil {
					t.Fatalf("mask=%d consultation=%d %s rejected valid choices: %v", mask, consultation, id, err)
				}
				selections = append(selections, selected...)
				next, ending, err := routes.Resolve(id, selections)
				wantNext := love2024Episode(testRioLoveID, fmt.Sprintf("%03d", index+2))
				if index == 6 {
					wantNext = love2024Episode(testRioLoveID, want)
				}
				if err != nil || ending != "" || next != wantNext {
					t.Fatalf("%d points (mask=%d consultation=%d) after %s: next=%s ending=%s err=%v want=%s", points, mask, consultation, id, next, ending, err, wantNext)
				}
			}
			seenTotals[points] = true
		}
	}
	for points := 0; points <= 18; points++ {
		if !seenTotals[points] {
			t.Fatalf("fixture missed attainable total %d, including route boundaries 0/1/2/10/11/18", points)
		}
	}
}

func TestLoveRioConditionalChoicesScenario(t *testing.T) {
	routes, err := moshikoi.LoadRoutes(testRioLoveID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		episode     string
		short, long []int32
	}{
		{"006", []int32{1, 1, 1}, []int32{1, 1, 2, 2}},
		{"008-happy", []int32{2}, []int32{1, 1}},
		{"008-normal", []int32{1}, []int32{2, 1}},
		{"008-bad", []int32{1}, []int32{2, 1}},
	} {
		id := love2024Episode(testRioLoveID, tc.episode)
		for _, choices := range [][]int32{tc.short, tc.long} {
			if _, err := routes.SelectChoices(id, love2024FinishRequest(id, choices...).GetSelectStoryChoiceInfos()); err != nil {
				t.Fatalf("%s rejected reachable choices %v: %v", id, choices, err)
			}
		}
		missing := love2024FinishRequest(id, tc.long[:len(tc.long)-1]...)
		if _, err := routes.SelectChoices(id, missing.GetSelectStoryChoiceInfos()); err == nil {
			t.Fatalf("%s accepted an omitted reachable conditional choice", id)
		}
		extra := love2024FinishRequest(id, append(slices.Clone(tc.short), 1)...)
		if _, err := routes.SelectChoices(id, extra.GetSelectStoryChoiceInfos()); err == nil {
			t.Fatalf("%s accepted an unreachable active choice", id)
		}
		extra.SelectStoryChoiceInfos[len(extra.SelectStoryChoiceInfos)-1].IsValid = false
		selected, err := routes.SelectChoices(id, extra.GetSelectStoryChoiceInfos())
		if err != nil || len(selected) != len(tc.short) {
			t.Fatalf("%s did not ignore unreachable historical choices: %v", id, err)
		}
	}
}

func TestLoveRioSaveLoadPointsIsolationScenario(t *testing.T) {
	setupTestUser(t)
	client, rio := registerRioLove(t)
	yu := prepareLove2024Series(t, testYuLoveID)
	for index, choices := range yuHappyPrefix()[:3] {
		playLove2024Episode(t, client, yu, fmt.Sprintf("%03d", index+1), 9, choices...)
	}
	playLove2024Episode(t, client, yu, "004", 9, 1, 2)
	playLove2024Episode(t, client, yu, "001", 9, 1, 1)
	loveCall(t, client.Save, &papi.LoveSaveRequest{LoveId: testYuLoveID, SlotNumber: 1})
	loveCall(t, client.UpdateSaveName, &papi.LoveUpdateSaveNameRequest{LoveId: testYuLoveID, SlotNumber: 1, Name: "Yu save"})
	yuBefore, yuLoveBefore := love2024State(t, testYuLoveID), storedLove2024(t, testYuLoveID)

	playLove2024Episode(t, client, rio, "001", 9, 1, 1, 2)
	playRioPrefix(t, client, rio, rioMaximumPointPrefix()[:5]) // Eleven points before EP6.
	loveCall(t, client.Save, &papi.LoveSaveRequest{LoveId: testRioLoveID, SlotNumber: 1})
	loveCall(t, client.UpdateSaveName, &papi.LoveUpdateSaveNameRequest{LoveId: testRioLoveID, SlotNumber: 1, Name: "Rio eleven"})
	savedState, savedLove := love2024State(t, testRioLoveID), storedLove2024(t, testRioLoveID)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testRioLoveID})
	playRioPrefix(t, client, rio, rioHappyPrefix()[:5])
	setLoveItem(t, rio.Config.PresentItemID, 1)
	loveCall(t, client.Present, &papi.LovePresentRequest{LoveId: testRioLoveID})
	pointsBefore := loveItemAmount(t, rio.Config.StoryReadItemID)
	loveCall(t, client.Load, &papi.LoveLoadRequest{LoveId: testRioLoveID, SlotNumber: 1})
	loaded, loadedLove := love2024State(t, testRioLoveID), storedLove2024(t, testRioLoveID)
	if !slices.Equal(loaded.Selections, savedState.Selections) || !slices.Equal(loaded.StoryEpisodeIDs, savedState.StoryEpisodeIDs) || loaded.Exp != savedState.Exp ||
		loadedLove.GetHiddenFavorability() != savedLove.GetHiddenFavorability()+rio.Config.PresentFavorability || !slices.Equal(loadedLove.GetUnlockedLoveStoryEndIDs(), savedLove.GetUnlockedLoveStoryEndIDs()) ||
		loveItemAmount(t, rio.Config.StoryReadItemID) != pointsBefore || loveItemAmount(t, rio.Config.PresentItemID) != 0 {
		t.Fatal("Rio Load did not restore route selections independently of permanent favorability, EXP and resources")
	}
	playLove2024Episode(t, client, rio, "006", 9, 1, 1, 2, 2)
	playLove2024Episode(t, client, rio, "007", 9, 1, 1)
	if !slices.Contains(love2024State(t, testRioLoveID).StoryEpisodeIDs, love2024Episode(testRioLoveID, "008-bad")) {
		t.Fatal("loading Rio's eleven-point choices lost the bad threshold or used gifted favorability instead")
	}
	yuAfter := love2024State(t, testYuLoveID)
	if !slices.Equal(yuBefore.Selections, yuAfter.Selections) || !slices.Equal(yuBefore.StoryEpisodeIDs, yuAfter.StoryEpisodeIDs) ||
		!proto.Equal(yuBefore.Saves[0].Info, yuAfter.Saves[0].Info) || !slices.Equal(yuBefore.Saves[0].Selections, yuAfter.Saves[0].Selections) ||
		!proto.Equal(yuLoveBefore, storedLove2024(t, testYuLoveID)) {
		t.Fatal("Rio's slot 1 or progression overwrote Yu's save or progress")
	}
	assertRioLoveVisibility(t)
}

func TestLoveRioConditionalFinishRollbackScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerRioLove(t)
	playRioPrefix(t, client, catalog, rioHappyPrefix()[:5])
	id := love2024Episode(testRioLoveID, "006")
	start := loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: id})
	if len(start.GetAdvBranchInfos()) != 0 {
		t.Fatal("Rio's choice-only script received unsolicited conditional branch information")
	}
	before := love2024State(t, testRioLoveID)
	loveReject(t, codes.InvalidArgument, client.Finish, love2024FinishRequest(id, 1, 1, 1, 1))
	loveReject(t, codes.InvalidArgument, client.Finish, love2024FinishRequest(id, 1, 1, 2))
	after := love2024State(t, testRioLoveID)
	if !storedLove2024(t, testRioLoveID).GetIsReadingStory() || !slices.Equal(before.Selections, after.Selections) || !slices.Equal(before.StoryEpisodeIDs, after.StoryEpisodeIDs) {
		t.Fatal("invalid conditional Finish mutated or ended Rio's active route")
	}
	// The client can echo the skipped alternative as history, not current input.
	request := love2024FinishRequest(id, 1, 1, 1, 1)
	request.SelectStoryChoiceInfos[3].IsValid = false
	loveCall(t, client.Finish, request)
	playLove2024Episode(t, client, catalog, "007", 9, 1, 1)
	id = love2024Episode(testRioLoveID, "008-happy") // Exactly one negative point.
	loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: id})
	loveReject(t, codes.InvalidArgument, client.Finish, love2024FinishRequest(id, 1))
	loveReject(t, codes.InvalidArgument, client.Finish, love2024FinishRequest(id, 2, 1))
	loveCall(t, client.Finish, love2024FinishRequest(id, 1, 2))
	if !slices.Contains(storedLove2024(t, testRioLoveID).GetUnlockedLoveStoryEndIDs(), "love-end-love-26-0313-008-happy-001") {
		t.Fatal("Rio conditional-choice correction did not finish the TRUE ending")
	}
}

func rioHappyPrefix() [][]int32 {
	return [][]int32{{1, 1, 1}, {1, 1, 1}, {2, 2}, {2, 1}, {2, 1, 1}, {1, 1, 2, 2}, {1, 1}}
}

func rioMaximumPointPrefix() [][]int32 {
	return [][]int32{{2, 1, 1}, {2, 2, 1}, {1, 1}, {1, 1}, {1, 1, 1}, {1, 2, 1}, {2, 2}}
}

func registerRioLove(t *testing.T) (papi.LoveClient, *moshikoi.Catalog) {
	t.Helper()
	client := registerLove2024Series(t)
	loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: testRioLoveID, Name: "Manager"})
	return client, prepareLove2024Series(t, testRioLoveID)
}

func playRioPrefix(t *testing.T, client papi.LoveClient, catalog *moshikoi.Catalog, path [][]int32) {
	t.Helper()
	for index, choices := range path {
		playLove2024Episode(t, client, catalog, fmt.Sprintf("%03d", index+1), 9, choices...)
	}
}

func assertRioLoveVisibility(t *testing.T) {
	t.Helper()
	assertLove2024Visibility(t)
	want := moshikoi.VisibleLove(storedLove2024(t, testRioLoveID))
	user := loveCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if !slices.ContainsFunc(user.GetLoves(), func(row *ptransaction.UserLove) bool { return proto.Equal(row, want) }) {
		t.Fatal("User.Get omitted Rio's stored Love progression")
	}
	home := loveCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	assertHomeLoveEntrances(t, home)
}
