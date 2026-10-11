package test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const testLoveID = "love-23-0514"

func TestLoveLessonAndRegistrationScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewLoveClient(testConfig.GrpcConn)
	catalog := loveCatalog(t)
	title := loveCall(t, client.Title, &papi.Empty{})
	assertLoveStandardTitle(t, title)
	if title.GetLoveInfos()[0].GetLoveId() != testLoveID ||
		title.GetLoveInfos()[0].GetEndTime() != 0 || !title.GetLoveInfos()[0].GetIsUnplayed() {
		t.Fatalf("fresh title did not expose the permanent events with Rei first: %v", title)
	}
	if !title.GetLoveInfos()[0].GetIsUnlocked() {
		loveReject(t, codes.FailedPrecondition, client.Register, &papi.LoveRegisterRequest{LoveId: testLoveID, Name: "Manager"})
	}
	unlockTestLove(t)
	loveReject(t, codes.NotFound, client.Register, &papi.LoveRegisterRequest{LoveId: "love-not-supported", Name: "Manager"})
	loveReject(t, codes.InvalidArgument, client.Register, &papi.LoveRegisterRequest{LoveId: testLoveID})
	registered := loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: testLoveID, Name: "Manager"})
	assertLoveDelta(t, registered.GetCommonResponse(), false)
	top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testLoveID})
	assertDummyNoticeInfo(t, "Love.Top standard", top.GetNoticeInfo())
	if top.GetPlayerName() != "Manager" || top.GetSaveInfo().GetUnlocked() ||
		!slices.Equal(top.GetStoryEpisodeIds(), []string{loveEpisode("001")}) {
		t.Fatalf("unexpected fresh Love.Top progression: %v", top)
	}
	loveCall(t, client.UpdateName, &papi.LoveUpdateNameRequest{LoveId: testLoveID, Name: "マネージャー"})
	if got := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testLoveID}).GetPlayerName(); got != "マネージャー" {
		t.Fatalf("player name was not persisted: %q", got)
	}
	character := loveCall(t, client.GetMainCharacter, &papi.LoveGetMainCharacterRequest{LoveId: testLoveID})
	if character.GetMainCharacterInfo().GetCharacterId() != catalog.Config.CharacterID {
		t.Fatalf("unexpected heroine: %v", character)
	}
	loveCall(t, client.TitleExtra, &papi.LoveTitleExtraRequest{LoveId: testLoveID})
	loveReject(t, codes.FailedPrecondition, client.Save, &papi.LoveSaveRequest{LoveId: testLoveID, SlotNumber: 1})
	assertLoveVisibility(t, false)

	// First clear is free, but advancing beyond the current level is not.
	lovePlayEpisode(t, client, "001", 2, 1)
	loveReject(t, codes.FailedPrecondition, client.Start, &papi.LoveStartRequest{StoryEpisodeId: loveEpisode("002")})
	state := loveState(t)
	state.LastLessonRewardTime = time.Now().Add(-48 * time.Hour).UnixMilli()
	storeLoveState(t, state)
	beforeItems := loveItemAmount(t, catalog.Config.StoryReadItemID)
	level := catalog.LevelData(state.Exp)
	steps := catalog.Config.LessonMaxHours * 3600 / catalog.Config.LessonStepSeconds
	expectedExp := steps * level.GetLessonStepRewardExp()
	expectedItems := steps * level.GetLessonStepRewardStoryReadItemAmount()
	reward := loveCall(t, client.ReceiveLessonReward, &papi.LoveReceiveLessonRewardRequest{LoveId: testLoveID})
	if reward.GetObtainExp() != expectedExp || reward.GetTotalExp() != state.Exp+expectedExp ||
		loveItemAmount(t, catalog.Config.StoryReadItemID) != beforeItems+expectedItems {
		t.Fatalf("48h absence did not cap rewards to the current 12h master rates: %v", reward)
	}
	if reward.GetLessonProgress().GetMaxRewardTime()-reward.GetLessonProgress().GetLastReceiveTime() != int64(12*time.Hour/time.Millisecond) {
		t.Fatalf("lesson reward window is not 12h: %v", reward.GetLessonProgress())
	}
	// Anchor at now to prove that a claim cannot award the previous absence twice.
	state = loveState(t)
	state.LastLessonRewardTime = time.Now().UnixMilli()
	storeLoveState(t, state)
	repeated := loveCall(t, client.ReceiveLessonReward, &papi.LoveReceiveLessonRewardRequest{LoveId: testLoveID})
	if repeated.GetObtainExp() != 0 || repeated.GetTotalExp() != state.Exp {
		t.Fatalf("immediate repeated claim awarded elapsed rewards twice: %v", repeated)
	}
	setLoveItem(t, catalog.Config.PromoteItemID, 0)
	loveReject(t, codes.ResourceExhausted, client.PromoteLesson, &papi.LovePromoteLessonRequest{LoveId: testLoveID})
	setLoveItem(t, catalog.Config.PromoteItemID, 1)
	state = loveState(t)
	beforeItems = loveItemAmount(t, catalog.Config.StoryReadItemID)
	level = catalog.LevelData(state.Exp)
	steps = catalog.Config.PromoteMinutes * 60 / catalog.Config.LessonStepSeconds
	drink := loveCall(t, client.PromoteLesson, &papi.LovePromoteLessonRequest{LoveId: testLoveID})
	if drink.GetObtainExp() != steps*level.GetLessonStepRewardExp() ||
		drink.GetTotalExp() != state.Exp+drink.GetObtainExp() ||
		loveItemAmount(t, catalog.Config.PromoteItemID) != 0 ||
		loveItemAmount(t, catalog.Config.StoryReadItemID) != beforeItems+steps*level.GetLessonStepRewardStoryReadItemAmount() {
		t.Fatalf("skip drink did not consume one item and grant the current 2h reward: %v", drink)
	}
	setLoveItem(t, catalog.Config.PresentItemID, 1)
	beforeLove := storedLove(t)
	present := loveCall(t, client.Present, &papi.LovePresentRequest{LoveId: testLoveID})
	if present.GetHiddenFavorability() != beforeLove.GetHiddenFavorability()+catalog.Config.PresentFavorability ||
		loveItemAmount(t, catalog.Config.PresentItemID) != 0 {
		t.Fatalf("present was not consumed or favorability was not persisted: %v", present)
	}
	loveReject(t, codes.ResourceExhausted, client.Present, &papi.LovePresentRequest{LoveId: testLoveID})
	assertLoveVisibility(t, false)
}

func TestLoveAllEndingRoutesScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerTestLove(t)
	raiseTestLoveLevel(t, catalog)
	setLoveItem(t, catalog.Config.StoryReadItemID, 1_000_000)

	// API choice indices are one-based; these paths independently encode the
	// supplied route map rather than calling the implementation's route resolver.
	type route struct {
		end     string
		last    string
		choices []int32
		normal  bool
		bad     bool
	}
	routes := []route{
		{end: "001-bad", last: "001", choices: []int32{2, 2}},
		{end: "008-happy-001", last: "008-happy", choices: []int32{1}},
		{end: "008-happy-002", last: "008-happy", choices: []int32{2}},
		{end: "008-happy-003", last: "008-happy", choices: []int32{3}},
		{end: "008-normal-001", last: "008-normal", choices: []int32{1}, normal: true},
		{end: "008-normal-002", last: "008-normal", choices: []int32{2}, normal: true},
		{end: "008-bad-001", last: "007-bad", choices: []int32{1}, bad: true},
		{end: "008-bad-002", last: "007-bad", choices: []int32{2}, bad: true},
		{end: "008-bad-003", last: "007-bad", choices: []int32{3}, bad: true},
	}
	for i, route := range routes {
		t.Run(route.end, func(t *testing.T) {
			if i != 0 {
				reset := loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testLoveID})
				if !slices.Equal(reset.GetStoryEpisodeIds(), []string{loveEpisode("001")}) {
					t.Fatalf("reset did not restore the opening: %v", reset)
				}
			}
			if route.last != "001" {
				lovePlayEpisode(t, client, "001", 2, 1)
				lovePlayEpisode(t, client, "002", 1, 2)
				if route.normal {
					// Missing only happy flag1 must divert an otherwise perfect route.
					lovePlayEpisode(t, client, "003", 2, 1)
				} else {
					lovePlayEpisode(t, client, "003", 1, 1)
				}
				lovePlayEpisode(t, client, "004", 1, 2)
				lovePlayEpisode(t, client, "005", 1, 2)
				if route.bad {
					lovePlayEpisode(t, client, "006", 2, 2, 2)
				} else {
					lovePlayEpisode(t, client, "006", 2, 2, 1)
					lovePlayEpisode(t, client, "007", 2, 1)
				}
			}
			top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testLoveID})
			if !slices.Contains(top.GetStoryEpisodeIds(), loveEpisode(route.last)) {
				t.Fatalf("route did not unlock expected terminal episode %q: %v", route.last, top.GetStoryEpisodeIds())
			}
			finish := lovePlayEpisode(t, client, route.last, route.choices...)
			love := storedLove(t)
			if !slices.Contains(love.GetUnlockedLoveStoryEndIDs(), "love-end-"+testLoveID+"-"+route.end) ||
				len(love.GetUnlockedLoveStoryEndIDs()) != i+1 || finish.GetStoryEndName() == "" || finish.GetUnlockSave() != (i == 0) {
				t.Fatalf("wrong ending/unlock state for %s: finish=%v love=%v", route.end, finish, love)
			}
		})
	}
	// Re-clearing an already recorded ending cannot award its permanent bonus
	// again, even though the client can replay it without paying story points.
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testLoveID})
	beforeRepeat := storedLove(t)
	lovePlayEpisode(t, client, "001", 2, 2)
	if !proto.Equal(beforeRepeat, storedLove(t)) {
		t.Fatal("repeating a recorded ending changed permanent favorability or ending history")
	}
	setLoveItem(t, catalog.Config.StoryReadItemID, 0)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testLoveID})
	lovePlayEpisode(t, client, "001", 2, 1)
	lovePlayEpisode(t, client, "002", 1, 1)
	if loveItemAmount(t, catalog.Config.StoryReadItemID) != 0 || len(storedLove(t).GetUnlockedLoveStoryEndIDs()) != len(routes) {
		t.Fatal("all-ending replay consumed story points or removed ending history")
	}
	assertLoveVisibility(t, false)
}

func TestLoveSaveLoadAndStoryCostScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerTestLove(t)
	raiseTestLoveLevel(t, catalog)
	lovePlayEpisode(t, client, "001", 2, 2) // Unlock saves with the early ending.
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testLoveID})
	lovePlayEpisode(t, client, "001", 2, 1)
	before := storedLove(t)
	loveCall(t, client.Save, &papi.LoveSaveRequest{LoveId: testLoveID, SlotNumber: 1})
	loveCall(t, client.UpdateSaveName, &papi.LoveUpdateSaveNameRequest{LoveId: testLoveID, SlotNumber: 1, Name: "Episode 2"})
	top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testLoveID})
	if !top.GetSaveInfo().GetUnlocked() || !slices.ContainsFunc(top.GetSaveInfo().GetSaveDataInfos(), func(info *papi.LoveSaveDataInfo) bool {
		return info.GetSlotNumber() == 1 && info.GetName() == "Episode 2" && info.GetSaveTime() > 0
	}) {
		t.Fatalf("save and renamed slot were not persisted: %v", top.GetSaveInfo())
	}
	episode := catalog.Episode(loveEpisode("002"))
	if episode == nil || episode.GetConsumptionItemAmount() <= 0 {
		t.Fatal("master does not contain a paid second episode")
	}
	cost := int64(episode.GetConsumptionItemAmount())
	setLoveItem(t, catalog.Config.StoryReadItemID, cost-1)
	loveReject(t, codes.ResourceExhausted, client.Start, &papi.LoveStartRequest{StoryEpisodeId: episode.GetId()})
	if storedLove(t).GetIsReadingStory() || loveItemAmount(t, catalog.Config.StoryReadItemID) != cost-1 {
		t.Fatal("rejected story start changed reading state or inventory")
	}
	setLoveItem(t, catalog.Config.StoryReadItemID, cost)
	start := loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: episode.GetId()})
	assertLoveDelta(t, start.GetCommonResponse(), true)
	loveCall(t, client.Restart, &papi.LoveRestartRequest{StoryEpisodeId: episode.GetId()})
	loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: episode.GetId()})
	if loveItemAmount(t, catalog.Config.StoryReadItemID) != 0 {
		t.Fatal("start/restart did not debit exactly one story price")
	}
	loveReject(t, codes.InvalidArgument, client.Finish, &papi.LoveFinishRequest{StoryEpisodeId: episode.GetId(), SelectStoryChoiceInfos: []*papi.LoveSelectStoryChoiceInfo{{GroupIndex: 1, Index: 99, IsValid: true}}})
	if !storedLove(t).GetIsReadingStory() {
		t.Fatal("invalid choices ended the active story")
	}
	// The official client sends past alternatives too. They must not overwrite
	// the true selections used for this playthrough.
	finishRequest := loveFinishRequest("002", 1, 2)
	finishRequest.SelectStoryChoiceInfos = append(finishRequest.SelectStoryChoiceInfos,
		&papi.LoveSelectStoryChoiceInfo{GroupIndex: 1, Index: 2, IsValid: false},
		&papi.LoveSelectStoryChoiceInfo{GroupIndex: 2, Index: 1, IsValid: false})
	loveCall(t, client.Finish, finishRequest)
	setLoveItem(t, catalog.Config.PresentItemID, 1)
	loveCall(t, client.Present, &papi.LovePresentRequest{LoveId: testLoveID})
	stateBeforeLoad := loveState(t)
	load := loveCall(t, client.Load, &papi.LoveLoadRequest{LoveId: testLoveID, SlotNumber: 1})
	if !slices.Equal(load.GetStoryEpisodeIds(), []string{loveEpisode("001"), loveEpisode("002")}) {
		t.Fatalf("load did not restore saved episode progression: %v", load)
	}
	after := storedLove(t)
	if after.GetHiddenFavorability() != before.GetHiddenFavorability()+catalog.Config.PresentFavorability ||
		!slices.Equal(after.GetUnlockedLoveStoryEndIDs(), before.GetUnlockedLoveStoryEndIDs()) ||
		loveState(t).Exp != stateBeforeLoad.Exp || loveItemAmount(t, catalog.Config.StoryReadItemID) != 0 {
		t.Fatal("load rolled back gifts, endings, EXP, or spent inventory instead of only the route")
	}
	// Loading does not make an unfinished ending collection free to replay.
	loveReject(t, codes.ResourceExhausted, client.Start, &papi.LoveStartRequest{StoryEpisodeId: episode.GetId()})
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testLoveID})
	loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: loveEpisode("001")})
	loveCall(t, client.ResetReadingStoryStatus, &papi.LoveResetReadingStoryStatusRequest{LoveId: testLoveID})
	if storedLove(t).GetIsReadingStory() {
		t.Fatal("ResetReadingStoryStatus left the user reading a story")
	}
	assertLoveVisibility(t, false)
}

func TestLoveGodReplayScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	client := papi.NewLoveClient(testConfig.GrpcConn)
	catalog := loveCatalog(t)
	top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testLoveID})
	if top.GetPlayerName() == "" || !top.GetSaveInfo().GetUnlocked() || len(storedLove(t).GetUnlockedLoveStoryEndIDs()) != 9 {
		t.Fatalf("god did not initialize completed, registered Love progress: %v", top)
	}
	if catalog.Level(top.GetExp()) != catalog.Levels[len(catalog.Levels)-1].GetLevel() {
		t.Fatalf("god Love level was not raised to the current master maximum: %d", top.GetExp())
	}
	setLoveItem(t, catalog.Config.StoryReadItemID, 0)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testLoveID})
	lovePlayEpisode(t, client, "001", 2, 1)
	lovePlayEpisode(t, client, "002", 1, 2)
	if loveItemAmount(t, catalog.Config.StoryReadItemID) != 0 {
		t.Fatal("god all-ending replay charged story points")
	}
	assertLoveVisibility(t, true)
	nagisaCatalog, err := moshikoi.LoadCatalog(context.Background(), testNagisaLoveID)
	if err != nil {
		t.Fatal(err)
	}
	nagisaTop := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testNagisaLoveID})
	if nagisaTop.GetPlayerName() == "" || !nagisaTop.GetSaveInfo().GetUnlocked() ||
		len(storedNagisaLove(t).GetUnlockedLoveStoryEndIDs()) != 8 ||
		nagisaCatalog.Level(nagisaTop.GetExp()) != nagisaCatalog.Levels[len(nagisaCatalog.Levels)-1].GetLevel() {
		t.Fatalf("god did not fully initialize the new Nagisa series: %v", nagisaTop)
	}
	setLoveItem(t, nagisaCatalog.Config.StoryReadItemID, 0)
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testNagisaLoveID})
	playNagisaEpisode(t, client, nagisaCatalog, "001", 1, 1)
	playNagisaEpisode(t, client, nagisaCatalog, "002", 1, 1)
	assertNagisaVisibility(t)
	for _, series := range []struct {
		id      string
		endings int
	}{{"love-24-0517", 8}, {"love-24-1114", 9}, {"love-26-0313", 9}, {"love-extra-25-0426", 12}} {
		catalog, err := moshikoi.LoadCatalog(context.Background(), series.id)
		if err != nil {
			t.Fatal(err)
		}
		top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: series.id})
		stored, err := moshikoi.UserLove(context.Background(), testConfig.TestUserSuid, series.id)
		if err != nil {
			t.Fatal(err)
		}
		if top.GetPlayerName() == "" || !top.GetSaveInfo().GetUnlocked() || len(stored.GetUnlockedLoveStoryEndIDs()) != series.endings ||
			catalog.Level(top.GetExp()) != catalog.Levels[len(catalog.Levels)-1].GetLevel() {
			t.Fatalf("god did not fully initialize %s: %v", series.id, top)
		}
		setLoveItem(t, catalog.Config.StoryReadItemID, 0)
		loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: series.id})
		for _, suffix := range []string{"001", "002"} {
			episodeID := moshikoi.EpisodeIDPrefix(series.id) + suffix
			loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: episodeID})
			choices := []*papi.LoveSelectStoryChoiceInfo{
				{GroupIndex: 1, Index: 1, IsValid: true}, {GroupIndex: 2, Index: 1, IsValid: true},
			}
			if series.id == "love-26-0313" {
				choices = append(choices, &papi.LoveSelectStoryChoiceInfo{GroupIndex: 3, Index: 1, IsValid: true})
			}
			if series.id == "love-extra-25-0426" {
				for group := int32(3); group <= 5; group++ {
					choices = append(choices, &papi.LoveSelectStoryChoiceInfo{GroupIndex: group, Index: 1, IsValid: true})
				}
			}
			loveCall(t, client.Finish, &papi.LoveFinishRequest{StoryEpisodeId: episodeID, SelectStoryChoiceInfos: choices})
		}
		if loveItemAmount(t, catalog.Config.StoryReadItemID) != 0 {
			t.Fatalf("god replay charged story points for %s", series.id)
		}
	}
}

func loveCatalog(t *testing.T) *moshikoi.Catalog {
	t.Helper()
	catalog, err := moshikoi.LoadCatalog(context.Background(), testLoveID)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func unlockTestLove(t *testing.T) {
	t.Helper()
	area, err := opmaster.FindArea(context.Background(), "area-main-002")
	if err != nil {
		t.Fatal(err)
	}
	err = opuser.UpsertManyForGod(context.Background(), uname.UserArea, testConfig.TestUserSuid,
		[]opuser.GodUpsert[*ptransaction.UserArea]{{Key: bson.M{"areaId": area.GetId()}, Data: &ptransaction.UserArea{
			AreaId: area.GetId(), ClearQuestCount: int32(len(area.GetQuestIds())), LastClearedTime: time.Now().UnixMilli(),
		}}}, true)
	if err != nil {
		t.Fatal(err)
	}
}

func registerTestLove(t *testing.T) (papi.LoveClient, *moshikoi.Catalog) {
	t.Helper()
	unlockTestLove(t)
	client := papi.NewLoveClient(testConfig.GrpcConn)
	loveCall(t, client.Register, &papi.LoveRegisterRequest{LoveId: testLoveID, Name: "Manager"})
	return client, loveCatalog(t)
}

func raiseTestLoveLevel(t *testing.T, catalog *moshikoi.Catalog) {
	t.Helper()
	state := loveState(t)
	state.Exp = catalog.Levels[len(catalog.Levels)-1].GetRequiredExp()
	storeLoveState(t, state)
}

func loveState(t *testing.T) *umodel.UserLoveState {
	t.Helper()
	state, err := opuser.FindUserLoveState(context.Background(), testConfig.TestUserSuid, testLoveID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func storeLoveState(t *testing.T, state *umodel.UserLoveState) {
	t.Helper()
	if err := opuser.UpsertUserLoveState(context.Background(), state); err != nil {
		t.Fatal(err)
	}
}

func storedLove(t *testing.T) *ptransaction.UserLove {
	t.Helper()
	loves, err := opuser.FindAllUserLoves(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, love := range loves {
		if love.GetLoveId() == testLoveID {
			return love
		}
	}
	t.Fatal("registered user has no persisted Rei Love")
	return nil
}

func setLoveItem(t *testing.T, itemID string, amount int64) {
	t.Helper()
	ctx := context.Background()
	items, err := opuser.FindUserItems(ctx, testConfig.TestUserSuid, itemID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if err := opuser.DeleteUserItem(ctx, testConfig.TestUserSuid, itemID, item.GetExpiredTime()); err != nil {
			t.Fatal(err)
		}
	}
	if amount > 0 {
		if err := opuser.UpsertUserItem(ctx, testConfig.TestUserSuid, &ptransaction.UserItem{ItemId: itemID, Amount: amount}); err != nil {
			t.Fatal(err)
		}
	}
}

func loveItemAmount(t *testing.T, itemID string) int64 {
	t.Helper()
	items, err := opuser.FindUserItems(context.Background(), testConfig.TestUserSuid, itemID)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, item := range items {
		if item.GetExpiredTime() == 0 || item.GetExpiredTime() > time.Now().UnixMilli() {
			total += item.GetAmount()
		}
	}
	return total
}

func loveEpisode(suffix string) string { return "love-event-story-" + testLoveID + "-" + suffix }

func loveFinishRequest(suffix string, choices ...int32) *papi.LoveFinishRequest {
	request := &papi.LoveFinishRequest{StoryEpisodeId: loveEpisode(suffix)}
	for i, choice := range choices {
		request.SelectStoryChoiceInfos = append(request.SelectStoryChoiceInfos, &papi.LoveSelectStoryChoiceInfo{
			GroupIndex: int32(i + 1), Index: choice, IsValid: true,
		})
	}
	return request
}

func lovePlayEpisode(t *testing.T, client papi.LoveClient, suffix string, choices ...int32) *papi.LoveFinishResponse {
	t.Helper()
	loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: loveEpisode(suffix)})
	response := loveCall(t, client.Finish, loveFinishRequest(suffix, choices...))
	assertLoveDelta(t, response.GetCommonResponse(), false)
	return response
}

func assertLoveDelta(t *testing.T, common *papi.CommonResponse, reading bool) {
	t.Helper()
	if !slices.ContainsFunc(common.GetUpdatedData().GetLoves(), func(love *ptransaction.UserLove) bool {
		return love.GetLoveId() == testLoveID && love.GetIsReadingStory() == reading && proto.Equal(love, storedLove(t))
	}) {
		t.Fatalf("missing persisted Love post-image with reading=%v: %v", reading, common)
	}
}

func assertLoveVisibility(t *testing.T, god bool) {
	t.Helper()
	expected := storedLove(t)
	user := loveCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if !slices.ContainsFunc(user.GetLoves(), func(love *ptransaction.UserLove) bool { return proto.Equal(love, expected) }) {
		t.Fatal("User.Get omitted or changed persisted Love progress")
	}
	home := loveCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	assertHomeLoveEntrances(t, home)
	if god && len(expected.GetUnlockedLoveStoryEndIDs()) != 9 {
		t.Fatal("god Love endings were lost during replay")
	}
}

func loveCall[Req, Resp any](t *testing.T, call func(context.Context, Req, ...grpc.CallOption) (Resp, error), req Req) Resp {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	response, err := call(ctx, req)
	if err != nil {
		t.Fatalf("Love scenario request %v failed: %v", req, err)
	}
	if common, ok := any(response).(interface{ GetCommonResponse() *papi.CommonResponse }); ok && common.GetCommonResponse() == nil {
		t.Fatalf("Love scenario request %v omitted commonResponse", req)
	}
	return response
}

func loveReject[Req, Resp any](t *testing.T, want codes.Code, call func(context.Context, Req, ...grpc.CallOption) (Resp, error), req Req) {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	_, err = call(ctx, req)
	if status.Code(err) != want {
		t.Fatalf("Love request %v returned %v, want %v", req, err, want)
	}
}
