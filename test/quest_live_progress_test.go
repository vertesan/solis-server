package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/protobuf/proto"
)

func TestQuestLiveProgressScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	number := questStartDeck(t, 1000000)
	areas, quests := questCatalogFixture(t)
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	tower := questAreasOfType(areas, penum.AreaType_AreaType_Tower)
	var first *papi.QuestStartResponse
	for _, q := range []*pmaster.Quest{quests[main[0].GetQuestIds()[0]], quests[tower[0].GetQuestIds()[0]]} {
		// Main clear EXP recomputes the fixture card stats; restore the intended
		// strong test lineup before each independent mode's successful clear.
		questRankSetAttributes(t, number, 1000000)
		result := questStartMeasured(t, client, &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 1})
		assertQuestProgressTrace(t, result)
		assertQuestSimpleThresholds(t, q, result.GetRankPatterns())
		if result.GetRankType() != penum.ResultRankType_ResultRankType_Unknown || result.GetRankPlus() != 0 || result.GetHighestRank() != 0 || len(result.GetReward().GetScoreRankRewards()) != 0 {
			t.Fatal("display thresholds introduced non-contest grade rewards or ranking")
		}
		replay := profileCall(t, client.GetLatestClearLiveResult, &papi.QuestGetLatestClearLiveResultRequest{QuestId: q.GetId(), UserId: testConfig.TestUserPublicId, IsDetail: true})
		assertQuestSimpleThresholds(t, q, replay.GetRankPatterns())
		if first == nil {
			first = result
		}
	}
	// A pre-fix replay with missing patterns should become viewable without
	// rewriting its historical trace or adding ranking/settlement side effects.
	legacy := proto.Clone(first.GetResult()).(*papi.LiveResult)
	if err := opuser.SaveQuestLiveResult(ctx, testConfig.TestUserSuid, first.GetQuestId(), legacy, true); err != nil {
		t.Fatal(err)
	}
	stored, err := opuser.FindQuestLiveResult(ctx, testConfig.TestUserSuid, first.GetQuestId(), opuser.QuestLiveReplayLatestClear)
	if err != nil || len(stored.GetRankPatterns()) != 0 {
		t.Fatalf("legacy replay fixture: %v", err)
	}
	for _, ranking := range []bool{false, true} {
		if ranking {
			r := profileCall(t, client.GetRankingLiveResult, &papi.QuestGetRankingLiveResultRequest{QuestId: first.GetQuestId(), UserId: testConfig.TestUserPublicId})
			assertQuestSimpleThresholds(t, quests[first.GetQuestId()], r.GetRankPatterns())
			if !proto.Equal(r.GetResult(), legacy) {
				t.Fatal("legacy ranking replay trace changed")
			}
		} else {
			r := profileCall(t, client.GetLatestClearLiveResult, &papi.QuestGetLatestClearLiveResultRequest{QuestId: first.GetQuestId(), UserId: testConfig.TestUserPublicId})
			assertQuestSimpleThresholds(t, quests[first.GetQuestId()], r.GetRankPatterns())
			if !proto.Equal(r.GetResult(), legacy) {
				t.Fatal("legacy clear replay trace changed")
			}
		}
	}
	_, daily := questStartDailyFixture(t)
	questRankSetAttributes(t, number, 1000000)
	dailyResult := questStartMeasured(t, client, &papi.QuestStartRequest{QuestId: daily.GetId(), DeckNumber: number, PlayCount: 1})
	assertQuestProgressTrace(t, dailyResult)
	if len(dailyResult.GetRankPatterns()) < 5 || dailyResult.GetRankType() < penum.ResultRankType_ResultRankType_C {
		t.Fatal("daily score grade progression regressed")
	}
	// NPC battle traces also retain the official three display markers at zero.
	var boss *pmaster.Quest
	for _, area := range main {
		for _, id := range area.GetQuestIds() {
			if quests[id].GetClearScore() == 0 {
				boss = quests[id]
				break
			}
		}
		if boss != nil {
			break
		}
		questSeedFullArea(t, area)
	}
	if boss == nil {
		t.Fatal("master lacks a main boss fixture")
	}
	maximum, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	questSetManagerLevel(t, maximum.GetLevel())
	questRankSetAttributes(t, number, 1000000)
	battle := questStartMeasured(t, client, &papi.QuestStartRequest{QuestId: boss.GetId(), DeckNumber: number, PlayCount: 1})
	assertQuestProgressTrace(t, battle)
	assertQuestSimpleThresholds(t, boss, battle.GetRankPatterns())
	if len(battle.GetResult().GetUserInfos()) != 2 {
		t.Fatal("boss fixture did not run a battle")
	}
}

func assertQuestSimpleThresholds(t *testing.T, q *pmaster.Quest, got []*papi.QuestRankPattern) {
	t.Helper()
	want := []*papi.QuestRankPattern{
		{RankType: penum.ResultRankType_ResultRankType_B, ThresholdScore: q.GetClearScore() * 12 / 10},
		{RankType: penum.ResultRankType_ResultRankType_C, ThresholdScore: q.GetClearScore()},
		{RankType: penum.ResultRankType_ResultRankType_D},
	}
	if len(got) != len(want) {
		t.Fatalf("expected three display thresholds, got %v", got)
	}
	for i := range want {
		if !proto.Equal(got[i], want[i]) {
			t.Fatalf("display threshold mismatch: got%v want%v", got, want)
		}
	}
}

func assertQuestProgressTrace(t *testing.T, response *papi.QuestStartResponse) {
	t.Helper()
	if len(response.GetRankPatterns()) == 0 {
		t.Fatal("live lacks score/grade progress markers")
	}
	var last int64
	for _, chart := range response.GetResult().GetCharts() {
		if len(chart.GetUserStatuses()) == 0 {
			t.Fatal("chart lacks cumulative score")
		}
		score := chart.GetUserStatuses()[0].GetTotalScore()
		if score < last {
			t.Fatal("score progression decreased")
		}
		last = score
	}
	if last <= 0 || last != response.GetResult().GetUserInfos()[0].GetUserResult().GetScore() {
		t.Fatal("per-chart score progress does not reach final score")
	}
}
