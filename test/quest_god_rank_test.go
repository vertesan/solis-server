package test

import (
	"context"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

func TestQuestGodRankingScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	ctx := context.Background()
	areas, quests := questCatalogFixture(t)
	areaTypes := make(map[string]penum.AreaType, len(areas))
	var dailyID, contestID string
	for _, area := range areas {
		areaTypes[area.GetId()] = area.GetType()
		if len(area.GetQuestIds()) == 0 {
			continue
		}
		if dailyID == "" && area.GetType() == penum.AreaType_AreaType_Daily {
			dailyID = area.GetQuestIds()[0]
		}
		if contestID == "" && area.GetType() == penum.AreaType_AreaType_Contest {
			contestID = area.GetQuestIds()[0]
		}
	}
	if dailyID == "" || contestID == "" {
		t.Fatal("god ranking scenario requires daily and contest quests in master data")
	}
	assertRanks := func(rows []*ptransaction.UserQuest) {
		t.Helper()
		seen := make(map[string]bool, len(rows))
		for _, row := range rows {
			master := quests[row.GetQuestId()]
			if master == nil {
				t.Fatalf("god created an unknown quest: %s", row.GetQuestId())
			}
			seen[row.GetQuestId()] = true
			if areaTypes[master.GetAreaId()] == penum.AreaType_AreaType_Contest {
				if row.GetHighestRank() != 1 {
					t.Fatalf("god contest %s lifetime rank = %d, want 1", row.GetQuestId(), row.GetHighestRank())
				}
				continue
			}
			if row.GetHighestRank() != 0 || row.GetCurrentRankingHighestRank() != 0 || row.GetCurrentRankingHighestScore() != 0 || row.GetCurrentRankingHighestScoreTime() != 0 || row.GetCurrentRankingSeasonID() != "" {
				t.Fatalf("non-contest quest contains contest ranking state: %v", row)
			}
		}
		for id := range quests {
			if !seen[id] {
				t.Fatalf("god omitted master quest %s", id)
			}
		}
	}
	rows, err := opuser.FindAllUserQuests(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertRanks(rows)

	// Reproduce old god data on a daily quest and an existing, worse contest
	// placement. Administrative rank repair must not replace personal records.
	personalBests := make(map[string]*ptransaction.UserQuest, 2)
	for _, id := range []string{dailyID, contestID} {
		row, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, id)
		if err != nil {
			t.Fatal(err)
		}
		row.HighestScore++
		row.HighestScoreTime = time.Now().Add(-24 * time.Hour).UnixMilli()
		row.HighestRank = 1
		if id == contestID {
			row.HighestRank = 6
		}
		row.CurrentRankingHighestRank = row.GetHighestRank()
		row.CurrentRankingHighestScore = row.GetHighestScore()
		row.CurrentRankingHighestScoreTime = row.GetHighestScoreTime()
		row.CurrentRankingSeasonID = "legacy-contest-season"
		if err := opuser.UpsertUserQuest(ctx, testConfig.TestUserSuid, row); err != nil {
			t.Fatal(err)
		}
		personalBests[id] = row
	}
	cards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil || len(cards) < 5 {
		t.Fatalf("god ranking scenario requires five owned cards: %v", err)
	}
	deck := &papi.LiveUserDeck{DeckName: "preserved ranking unit"}
	for i, card := range cards[:5] {
		deck.Cards = append(deck.Cards, &papi.LiveDeckCard{
			Index: int32(i + 1), CardId: card.GetCardId(), Level: card.GetLevel(), Rarity: card.GetRarity(),
		})
	}
	result := &papi.LiveResult{Cleared: true, UserInfos: []*papi.LiveUserInfo{{
		Index: 1, UserId: testConfig.TestUserPublicId, UserDeck: deck,
		UserResult: &papi.LiveUserResult{Score: personalBests[contestID].GetHighestScore()},
	}}}
	if err := opuser.SaveQuestLiveResult(ctx, testConfig.TestUserSuid, contestID, result, true); err != nil {
		t.Fatal(err)
	}
	historyBefore, err := opuser.FindUserQuestLiveHistory(ctx, testConfig.TestUserSuid, contestID)
	if err != nil {
		t.Fatal(err)
	}

	promoteTestUser(t)
	rows, err = opuser.FindAllUserQuests(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertRanks(rows)
	for _, row := range rows {
		if before := personalBests[row.GetQuestId()]; before != nil {
			if row.GetHighestScore() != before.GetHighestScore() || row.GetHighestScoreTime() != before.GetHighestScoreTime() || row.GetHighestScoreRank() != before.GetHighestScoreRank() {
				t.Fatalf("rank repair changed personal score history: before=%v after=%v", before, row)
			}
		}
	}
	historyAfter, err := opuser.FindUserQuestLiveHistory(ctx, testConfig.TestUserSuid, contestID)
	if err != nil || *historyAfter != *historyBefore {
		t.Fatalf("rank repair replaced retained replay references: %v", err)
	}
	replay, err := opuser.FindQuestLiveResult(ctx, testConfig.TestUserSuid, contestID, opuser.QuestLiveReplayBest)
	if err != nil || !proto.Equal(replay.GetResult(), result) {
		t.Fatalf("rank repair changed the historical live or its unit: %v", err)
	}
	get := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	assertRanks(get.GetQuests())
	top := profileCall(t, papi.NewQuestClient(testConfig.GrpcConn).Top, &papi.Empty{})
	for _, area := range top.GetDailyAreas() {
		for _, quest := range area.GetQuests() {
			if quest.GetHighestRank() != 0 {
				t.Fatalf("daily catalog still advertises a contest placement: %v", quest)
			}
		}
	}
	ranking := profileCall(t, papi.NewQuestClient(testConfig.GrpcConn).ListContestRanking, &papi.QuestListContestRankingRequest{QuestId: contestID})
	if len(ranking.GetRanks()) != 1 || ranking.GetRanks()[0].GetDeckName() != deck.GetDeckName() || ranking.GetSelfScore() != personalBests[contestID].GetHighestScore() {
		t.Fatal("rank repair lost the contest's historical score or unit")
	}
}
