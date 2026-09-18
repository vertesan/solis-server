package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

// HighestRank at the response root is the pre-attempt lifetime rank. The
// client compares it with updatedData.quests to decide whether to show a
// ranking promotion; it is not an unconditional popup flag or the new rank.
func TestQuestContestRankPopupScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	areas, quests := questCatalogFixture(t)
	contests := questAreasOfType(areas, penum.AreaType_AreaType_Contest)
	if len(contests) == 0 || len(contests[0].GetQuestIds()) == 0 {
		t.Fatal("contest ranking scenario requires a contest quest")
	}
	q := quests[contests[0].GetQuestIds()[0]]
	condition, err := opmaster.FindCondition(ctx, q.GetUnlockConditionId())
	if err != nil {
		t.Fatal(err)
	}
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	var requiredOrder int32
	for _, setting := range condition.GetSettings() {
		for _, area := range main {
			if area.GetId() == setting.GetClearArea().GetAreaId() {
				requiredOrder = max(requiredOrder, area.GetOrder())
			}
		}
	}
	for _, area := range main {
		if area.GetOrder() <= requiredOrder {
			questSeedFullArea(t, area)
		}
	}
	maximum, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	questSetManagerLevel(t, maximum.GetLevel())
	tickets, err := opmaster.FindQuestTicketItems(ctx, penum.ItemType_ItemType_QuestContestTicket)
	if err != nil || len(tickets) != 1 {
		t.Fatalf("contest ticket fixture: tickets=%d error=%v", len(tickets), err)
	}
	setCardTrainingItem(t, tickets[0].GetId(), int64(q.GetTicketAmount())*10)
	number := questStartDeck(t, 0)
	request := &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 1, IsSkip: true}
	failed := questStartMeasured(t, client, request)
	if failed.GetResult().GetCleared() {
		t.Fatal("zero-attribute contest unexpectedly cleared")
	}
	assertQuestRankTransition(t, failed, 0, 0)
	if len(failed.GetReward().GetAchievedRankRewards()) != 0 {
		t.Fatal("failed first attempt granted a ranking milestone")
	}

	questRankSetAttributes(t, number, 1000000)
	first := questStartMeasured(t, client, request)
	if !first.GetResult().GetCleared() {
		t.Fatal("first contest clear fixture did not clear")
	}
	assertQuestRankTransition(t, first, 0, 1)
	stored := questRankStored(t, q.GetId())
	if stored.GetHighestRank() != 1 || stored.GetCurrentRankingHighestRank() != 1 {
		t.Fatal("contest lifetime/current rank did not persist")
	}
	seasons, err := opmaster.FindAllQuestContestRankingSeasons(ctx)
	if err != nil {
		t.Fatal(err)
	}
	activeSeason, activeStart := "", int64(-1)
	for _, season := range seasons {
		if season.GetStartTime() <= stored.GetLastClearedTime() && season.GetStartTime() > activeStart {
			activeSeason, activeStart = season.GetId(), season.GetStartTime()
		}
	}
	if activeSeason == "" || stored.GetCurrentRankingSeasonID() != activeSeason {
		t.Fatalf("contest record lacks current master season: %q want%q", stored.GetCurrentRankingSeasonID(), activeSeason)
	}
	if len(q.GetAchievedRankRewards()) > 0 && len(first.GetReward().GetAchievedRankRewards()) == 0 {
		t.Fatal("first contest clear omitted its ranking milestone reward")
	}

	// Viewing the saved best must not mutate/re-award the lifetime milestone.
	replay := profileCall(t, client.GetRankingLiveResult, &papi.QuestGetRankingLiveResultRequest{
		QuestId: q.GetId(), UserId: testConfig.TestUserPublicId,
	})
	if !proto.Equal(replay.GetResult(), first.GetResult()) || !proto.Equal(stored, questRankStored(t, q.GetId())) {
		t.Fatal("reading the ranking replay changed quest progress or its stored trace")
	}

	questRankSetAttributes(t, number, 10000000)
	improved := questStartMeasured(t, client, request)
	assertQuestRankTransition(t, improved, 1, 1)
	if !improved.GetResult().GetCleared() || improved.GetHighestScore() <= first.GetHighestScore() {
		t.Fatal("stronger contest lineup did not establish a new score best")
	}
	if len(improved.GetReward().GetAchievedRankRewards()) != 0 {
		t.Fatal("improving score re-awarded an already reached rank")
	}

	// A current-period rank can fall without losing the lifetime milestone.
	// Returning to rank one must compare against that lifetime best, not six.
	declined := questRankStored(t, q.GetId())
	declined.CurrentRankingHighestRank = 6
	if err := opuser.UpsertUserQuest(ctx, testConfig.TestUserSuid, declined); err != nil {
		t.Fatal(err)
	}
	questRankSetAttributes(t, number, 10000000)
	regained := questStartMeasured(t, client, request)
	assertQuestRankTransition(t, regained, 1, 1)
	if !regained.GetResult().GetCleared() || len(regained.GetReward().GetAchievedRankRewards()) != 0 {
		t.Fatal("regaining a previously reached rank repeated its milestone")
	}
	// Reproduce the real-device record: a prior private clear has lifetime rank
	// one and a high score but no season ID. Lower scores must retain the record
	// while publishing the matching season, rather than acting like a new rank.
	legacy := questRankStored(t, q.GetId())
	legacy.CurrentRankingSeasonID = ""
	if err := opuser.UpsertUserQuest(ctx, testConfig.TestUserSuid, legacy); err != nil {
		t.Fatal(err)
	}
	questRankSetAttributes(t, number, 1000000)
	repeated := questStartMeasured(t, client, request)
	assertQuestRankTransition(t, repeated, 1, 1)
	repaired := questRankStored(t, q.GetId())
	if !repeated.GetResult().GetCleared() || repeated.GetResult().GetUserInfos()[0].GetUserResult().GetScore() >= legacy.GetHighestScore() ||
		repaired.GetHighestScore() != legacy.GetHighestScore() || repaired.GetCurrentRankingHighestScore() != legacy.GetCurrentRankingHighestScore() ||
		repaired.GetCurrentRankingSeasonID() != activeSeason || len(repeated.GetReward().GetAchievedRankRewards()) != 0 {
		t.Fatal("lower-score repeat lost season/lifetime identity or repeated rank reward")
	}
	// A real season change clears only seasonal state, not lifetime rank or
	// high score. The new season's first score may be lower than lifetime best.
	priorSeason := proto.Clone(repaired).(*ptransaction.UserQuest)
	priorSeason.CurrentRankingSeasonID = "past-fixture-season"
	if err := opuser.UpsertUserQuest(ctx, testConfig.TestUserSuid, priorSeason); err != nil {
		t.Fatal(err)
	}
	seasonClear := questStartMeasured(t, client, request)
	assertQuestRankTransition(t, seasonClear, 1, 1)
	seasonState := questRankStored(t, q.GetId())
	if seasonState.GetCurrentRankingSeasonID() != activeSeason || seasonState.GetHighestScore() != priorSeason.GetHighestScore() ||
		seasonState.GetCurrentRankingHighestScore() != seasonClear.GetResult().GetUserInfos()[0].GetUserResult().GetScore() ||
		seasonState.GetCurrentRankingHighestScore() >= seasonState.GetHighestScore() || len(seasonClear.GetReward().GetAchievedRankRewards()) != 0 {
		t.Fatal("season rollover mixed lifetime/season score or repeated lifetime rank reward")
	}

	beforeFailure := questRankStored(t, q.GetId())
	questRankSetAttributes(t, number, 0)
	failed = questStartMeasured(t, client, request)
	assertQuestRankTransition(t, failed, 1, 1)
	if failed.GetResult().GetCleared() || len(failed.GetReward().GetAchievedRankRewards()) != 0 || !proto.Equal(beforeFailure, questRankStored(t, q.GetId())) {
		t.Fatal("failed contest changed prior rank/score or granted milestone rewards")
	}
}

func TestQuestNonContestRankPopupScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewQuestClient(testConfig.GrpcConn)
	number := questStartDeck(t, 1000000)
	areas, quests := questCatalogFixture(t)
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	tower := questAreasOfType(areas, penum.AreaType_AreaType_Tower)
	if len(main) == 0 || len(tower) == 0 {
		t.Fatal("non-contest scenario requires main and tower areas")
	}
	ids := []string{main[0].GetQuestIds()[0], tower[0].GetQuestIds()[0]}
	for _, id := range ids {
		questRankSetAttributes(t, number, 1000000)
		questRankSeedLegacyNonContest(t, id)
		result := questStartMeasured(t, client, &papi.QuestStartRequest{QuestId: id, DeckNumber: number, PlayCount: 1, IsSkip: true})
		if !result.GetResult().GetCleared() {
			t.Fatalf("non-contest fixture %s did not clear (target %d)", id, quests[id].GetClearScore())
		}
		assertQuestRankTransition(t, result, 0, 0)
		if stored := questRankStored(t, id); stored.GetHighestRank() != 0 || stored.GetCurrentRankingHighestRank() != 0 {
			t.Fatalf("non-contest %s retained legacy ranking fields: %v", id, stored)
		}
	}
	_, daily := questStartDailyFixture(t)
	questRankSetAttributes(t, number, 1000000)
	questRankSeedLegacyNonContest(t, daily.GetId())
	request := &papi.QuestStartRequest{QuestId: daily.GetId(), DeckNumber: number, PlayCount: 1, IsSkip: true}
	result := questStartMeasured(t, client, request)
	if !result.GetResult().GetCleared() {
		t.Fatal("daily fixture did not clear")
	}
	assertQuestRankTransition(t, result, 0, 0)
	stored := questRankStored(t, daily.GetId())
	if stored.GetHighestRank() != 0 || stored.GetCurrentRankingHighestRank() != 0 {
		t.Fatal("daily clear retained legacy ranking fields")
	}
	questRankSetAttributes(t, number, 0)
	failed := questStartMeasured(t, client, request)
	assertQuestRankTransition(t, failed, 0, 0)
	if failed.GetResult().GetCleared() || !proto.Equal(stored, questRankStored(t, daily.GetId())) {
		t.Fatal("failed daily live changed previously cleared progress")
	}
}

func assertQuestRankTransition(t *testing.T, response *papi.QuestStartResponse, previous, next int64) {
	t.Helper()
	if int64(response.GetHighestRank()) != previous {
		t.Fatalf("root highestRank=%d, want pre-attempt lifetime best %d", response.GetHighestRank(), previous)
	}
	for _, row := range response.GetCommonResponse().GetUpdatedData().GetQuests() {
		if row.GetQuestId() != response.GetQuestId() {
			continue
		}
		if row.GetHighestRank() != next || row.GetCurrentRankingHighestRank() != next {
			t.Fatalf("updated rank lifetime/current=%d/%d, want %d", row.GetHighestRank(), row.GetCurrentRankingHighestRank(), next)
		}
		return
	}
	t.Fatal("Start omitted updated quest rank")
}

func questRankStored(t *testing.T, id string) *ptransaction.UserQuest {
	t.Helper()
	row, err := opuser.FindUserQuest(context.Background(), testConfig.TestUserSuid, id)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func questRankSetAttributes(t *testing.T, number int32, value int64) {
	t.Helper()
	for _, card := range questStartUserDeckCards(t, number) {
		card.Vocal, card.Dance, card.Visual = value, value, value
		card.BaseVocal, card.BaseDance, card.BaseVisual = value, value, value
		if err := opuser.UpsertUserCard(context.Background(), testConfig.TestUserSuid, card); err != nil {
			t.Fatal(err)
		}
	}
}

func questRankSeedLegacyNonContest(t *testing.T, id string) {
	t.Helper()
	if err := opuser.UpsertUserQuest(context.Background(), testConfig.TestUserSuid, &ptransaction.UserQuest{
		QuestId: id, HighestRank: 1, CurrentRankingHighestRank: 1,
	}); err != nil {
		t.Fatal(err)
	}
}
