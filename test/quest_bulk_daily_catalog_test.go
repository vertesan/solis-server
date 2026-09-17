package test

import (
	"context"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

func TestQuestBulkDailyCatalogScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	deckClient := papi.NewDeckClient(testConfig.GrpcConn)
	number := questStartDeck(t, 1000000)
	area, q := questStartDailyFixture(t)
	if len(profileCall(t, client.ListDaily, &papi.Empty{}).GetBulkDailyInfos()) != 0 {
		t.Fatal("fresh daily menu fabricated a previous clear unit")
	}
	clear := questStartMeasured(t, client, &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 1, IsSkip: true})
	if !clear.GetResult().GetCleared() {
		t.Fatal("bulk-menu fixture did not clear the daily live")
	}
	replay, err := opuser.FindQuestLiveReplayInfo(ctx, testConfig.TestUserSuid, q.GetId(), opuser.QuestLiveReplayLatestClear)
	if err != nil || replay.DeckNumber != number || len(replay.DeckPositions) != 5 {
		t.Fatalf("successful daily clear did not retain its formation: %v, %v", replay, err)
	}
	assertMenu := func(expected []*ptransaction.UserDeckPosition) {
		t.Helper()
		daily := profileCall(t, client.ListDaily, &papi.Empty{})
		top := profileCall(t, client.Top, &papi.Empty{})
		if len(expected) == 0 {
			if len(daily.GetBulkDailyInfos()) != 0 || len(top.GetBulkDailyInfos()) != 0 {
				t.Fatal("unavailable bulk daily was still advertised")
			}
			return
		}
		if len(daily.GetBulkDailyInfos()) != 1 || len(top.GetBulkDailyInfos()) != 1 {
			t.Fatalf("expected one bulk daily, ListDaily=%d Top=%d", len(daily.GetBulkDailyInfos()), len(top.GetBulkDailyInfos()))
		}
		info := daily.GetBulkDailyInfos()[0]
		if !proto.Equal(info, top.GetBulkDailyInfos()[0]) || info.GetQuestId() != q.GetId() || len(info.GetDeckPositions()) != len(expected) {
			t.Fatal("Top and ListDaily disagree about the last cleared daily unit")
		}
		for _, position := range expected {
			got := info.GetDeckPositions()[position.GetPosition()-1]
			if got.GetPosition() != position.GetPosition() || got.GetCardId() != position.GetCardId() {
				t.Fatal("bulk menu lost the latest successfully cleared formation")
			}
		}
	}
	positions, err := opuser.FindUserDeckPositions(ctx, testConfig.TestUserSuid, number)
	if err != nil || len(positions) != 5 {
		t.Fatalf("saved unit: %v", err)
	}
	original := make([]*ptransaction.UserDeckPosition, len(positions))
	for i, position := range positions {
		original[i] = proto.Clone(position).(*ptransaction.UserDeckPosition)
	}
	assertMenu(original)
	positions[0].CardId, positions[1].CardId = positions[1].GetCardId(), positions[0].GetCardId()
	profileCall(t, deckClient.Save, &papi.DeckSaveRequest{Number: number, Name: "更新したユニット", UserDeckPositions: positions})
	assertMenu(original)
	// Later unit edits, including removing a slot or deleting the unit entirely,
	// must not overwrite the independently retained successful formation.
	profileCall(t, deckClient.Save, &papi.DeckSaveRequest{Number: number, Name: "編集中", UserDeckPositions: positions[:4]})
	assertMenu(original)
	profileCall(t, deckClient.Save, &papi.DeckSaveRequest{Number: number, Name: "更新したユニット", UserDeckPositions: positions})
	assertMenu(original)
	questSeedArea(t, area, 1, 5, time.Now())
	assertMenu(nil)
	questSeedArea(t, area, 1, 5, time.Now().Add(-24*time.Hour))
	assertMenu(original)
	if err := opuser.DeleteUserDeck(ctx, testConfig.TestUserSuid, number); err != nil {
		t.Fatal(err)
	}
	assertMenu(original)
}
