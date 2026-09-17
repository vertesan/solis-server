package test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestQuestBulkDailyMixedPassScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	number := questStartDeck(t, 1000000)
	areas, quests := bulkDailyFixture(t)
	coinFirst, coinHighest := quests[areas[0].GetQuestIds()[0]], quests[areas[0].GetQuestIds()[1]]
	piece, film := quests[areas[1].GetQuestIds()[0]], quests[areas[2].GetQuestIds()[0]]
	for _, q := range []*pmaster.Quest{coinFirst, coinHighest, piece, film} {
		bulkDailyClear(t, client, number, q)
	}
	coinReplay, err := opuser.FindQuestLiveReplayInfo(ctx, testConfig.TestUserSuid, coinHighest.GetId(), opuser.QuestLiveReplayLatestClear)
	if err != nil {
		t.Fatal(err)
	}
	positions, err := opuser.FindUserDeckPositions(ctx, testConfig.TestUserSuid, number)
	if err != nil {
		t.Fatal(err)
	}
	positions[0].CardId, positions[1].CardId = positions[1].GetCardId(), positions[0].GetCardId()
	profileCall(t, papi.NewDeckClient(testConfig.GrpcConn).Save, &papi.DeckSaveRequest{
		Number: number, Name: "別ライブ用の編成", UserDeckPositions: positions,
	})
	// A more recent clear of a lower difficulty must not replace the highest
	// cleared quest or its independently saved formation.
	bulkDailyClear(t, client, number, coinFirst)
	for i, used := range []int32{0, 2, 5} {
		clearCount := int32(1)
		if i == 0 {
			clearCount = 2
		}
		questSeedArea(t, areas[i], clearCount, used, time.Now())
	}
	tickets := bulkDailySeedTickets(t, areas)
	before := bulkDailyUser(t)
	result := profileCall(t, client.StartBulkDaily, &papi.Empty{})
	if len(result.GetResults()) != 2 {
		t.Fatalf("mixed FREE passes produced %d results, want two", len(result.GetResults()))
	}
	wanted := map[string]int32{coinHighest.GetId(): 5, piece.GetId(): 3}
	var expectedExp int64
	for _, row := range result.GetResults() {
		count, exists := wanted[row.GetQuestId()]
		if !exists || row.GetPlayCount() != count || row.GetRankType() < penum.ResultRankType_ResultRankType_C || len(row.GetReward().GetClearRewards()) == 0 {
			t.Fatalf("unexpected bulk result: %v", row)
		}
		delete(wanted, row.GetQuestId())
		expectedExp += int64(quests[row.GetQuestId()].GetRewardManagerExp()) * int64(count)
		if row.GetQuestId() == coinHighest.GetId() {
			if len(row.GetDeckPositions()) != len(coinReplay.DeckPositions) {
				t.Fatal("bulk lost the latest successful quest formation")
			}
			for _, position := range coinReplay.DeckPositions {
				got := row.GetDeckPositions()[position.GetPosition()-1]
				if got.GetPosition() != position.GetPosition() || got.GetCardId() != position.GetCardId() {
					t.Fatal("later unit edit or lower-difficulty clear overwrote the bulk formation")
				}
			}
		}
	}
	after := bulkDailyUser(t)
	if len(wanted) != 0 || after.GetManagerExp()-before.GetManagerExp() != expectedExp || result.GetCommonResponse().GetUpdatedData().GetUser().GetManagerExp() != after.GetManagerExp() {
		t.Fatal("bulk manager EXP or final common response is not count-scaled across areas")
	}
	for _, area := range areas {
		bulkDailyAssertAreaCount(t, area.GetId(), 5)
	}
	for id, count := range tickets {
		if questStartUsablePasses(t, id) != count {
			t.Fatal("bulk daily consumed an owned ticket")
		}
	}
	bulkDailyAssertHidden(t, client)
	empty := profileCall(t, client.StartBulkDaily, &papi.Empty{})
	if len(empty.GetResults()) != 0 || !proto.Equal(after, bulkDailyUser(t)) {
		t.Fatal("repeating exhausted bulk dailies granted rewards again")
	}
}

func TestQuestBulkDailyCurrentStatsAndFailureScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	areas, quests := bulkDailyFixture(t)
	coin, piece := quests[areas[0].GetQuestIds()[0]], quests[areas[1].GetQuestIds()[0]]
	cards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil || len(cards) < 10 {
		t.Fatalf("configured test user needs two disjoint five-card units: cards=%d, err=%v", len(cards), err)
	}
	coinDeck := bulkDailySaveDeck(t, cards[:5])
	pieceDeck := bulkDailySaveDeck(t, cards[5:10])
	bulkDailyClear(t, client, coinDeck, coin)
	bulkDailyClear(t, client, pieceDeck, piece)
	for _, area := range areas[:2] {
		questSeedArea(t, area, 1, 0, time.Now())
	}
	beforeQuest, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, coin.GetId())
	if err != nil {
		t.Fatal(err)
	}
	beforeClear, err := opuser.FindQuestLiveReplayInfo(ctx, testConfig.TestUserSuid, coin.GetId(), opuser.QuestLiveReplayLatestClear)
	if err != nil {
		t.Fatal(err)
	}
	bulkDailySetAttributes(t, cards[:5], 0)
	before := bulkDailyUser(t)
	result := profileCall(t, client.StartBulkDaily, &papi.Empty{})
	if len(result.GetResults()) != 2 {
		t.Fatal("a failed area suppressed another valid bulk daily")
	}
	for _, row := range result.GetResults() {
		if row.GetQuestId() == coin.GetId() {
			if row.GetRankType() >= penum.ResultRankType_ResultRankType_C || len(row.GetReward().GetClearRewards())+len(row.GetReward().GetScoreRankRewards()) != 0 {
				t.Fatal("bulk reused the historical card stats/rank or rewarded a failed live")
			}
		} else if row.GetQuestId() != piece.GetId() || row.GetRankType() < penum.ResultRankType_ResultRankType_C {
			t.Fatalf("unexpected successful bulk area: %v", row)
		}
	}
	bulkDailyAssertAreaCount(t, coin.GetAreaId(), 0)
	bulkDailyAssertAreaCount(t, piece.GetAreaId(), 5)
	if bulkDailyUser(t).GetManagerExp()-before.GetManagerExp() != int64(piece.GetRewardManagerExp())*5 {
		t.Fatal("failed bulk area granted manager EXP")
	}
	afterQuest, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, coin.GetId())
	if err != nil || !proto.Equal(beforeQuest, afterQuest) {
		t.Fatalf("failed bulk daily modified clear/high-score history: %v", err)
	}
	afterClear, err := opuser.FindQuestLiveReplayInfo(ctx, testConfig.TestUserSuid, coin.GetId(), opuser.QuestLiveReplayLatestClear)
	if err != nil || afterClear.ID != beforeClear.ID {
		t.Fatalf("failed bulk daily discarded the usable last-clear formation: %v", err)
	}
	menu := profileCall(t, client.ListDaily, &papi.Empty{})
	if len(menu.GetBulkDailyInfos()) != 1 || menu.GetBulkDailyInfos()[0].GetQuestId() != coin.GetId() {
		t.Fatal("failed area's unspent FREE passes disappeared from the menu")
	}
	// No new Deck.Save or ordinary clear: improvements to the same owned cards
	// must be observed when preparing the retained formation on the next call.
	bulkDailySetAttributes(t, cards[:5], 1000000)
	retry := profileCall(t, client.StartBulkDaily, &papi.Empty{})
	if len(retry.GetResults()) != 1 || retry.GetResults()[0].GetQuestId() != coin.GetId() || retry.GetResults()[0].GetRankType() < penum.ResultRankType_ResultRankType_C || retry.GetResults()[0].GetPlayCount() != 5 {
		t.Fatal("improved current card stats did not clear the retained bulk formation")
	}
	bulkDailyAssertAreaCount(t, coin.GetAreaId(), 5)
	bulkDailyAssertHidden(t, client)
}

func TestQuestBulkDailyAtomicScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	number := questStartDeck(t, 1000000)
	areas, quests := bulkDailyFixture(t)
	coin, piece := quests[areas[0].GetQuestIds()[0]], quests[areas[1].GetQuestIds()[0]]
	for _, q := range []*pmaster.Quest{coin, piece} {
		bulkDailyClear(t, client, number, q)
	}
	for _, area := range areas[:2] {
		questSeedArea(t, area, 1, 0, time.Now())
	}
	before := bulkDailyUser(t)
	goldBefore, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	coinBefore, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, coin.GetId())
	if err != nil {
		t.Fatal(err)
	}
	clearBefore, err := opuser.FindQuestLiveReplayInfo(ctx, testConfig.TestUserSuid, coin.GetId(), opuser.QuestLiveReplayLatestClear)
	if err != nil {
		t.Fatal(err)
	}
	// Coin settles first. Overflowing the later piece reward must roll back
	// earlier rewards, FREE consumption and even the newly stored replay.
	pieceAmount := userItemAmount(t, "item-card_enhance")
	setCardTrainingItem(t, "item-card_enhance", math.MaxInt64)
	profileReject(t, codes.FailedPrecondition, client.StartBulkDaily, &papi.Empty{})
	goldAfter, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	coinAfter, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, coin.GetId())
	if err != nil {
		t.Fatal(err)
	}
	clearAfter, err := opuser.FindQuestLiveReplayInfo(ctx, testConfig.TestUserSuid, coin.GetId(), opuser.QuestLiveReplayLatestClear)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, bulkDailyUser(t)) || !proto.Equal(goldBefore, goldAfter) || !proto.Equal(coinBefore, coinAfter) || clearBefore.ID != clearAfter.ID || userItemAmount(t, "item-card_enhance") != math.MaxInt64 {
		t.Fatal("later bulk reward failure did not roll back the whole batch")
	}
	for _, area := range areas[:2] {
		bulkDailyAssertAreaCount(t, area.GetId(), 0)
	}
	setCardTrainingItem(t, "item-card_enhance", pieceAmount)
	type answer struct {
		result *papi.QuestStartBulkDailyResponse
		err    error
	}
	answers := make(chan answer, 2)
	for range 2 {
		rpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			defer cancel()
			result, err := client.StartBulkDaily(rpcCtx, &papi.Empty{})
			answers <- answer{result, err}
		}()
	}
	var completed, exhausted int
	for range 2 {
		a := <-answers
		if a.err != nil {
			t.Fatalf("concurrent bulk daily: %v", a.err)
		}
		switch len(a.result.GetResults()) {
		case 0:
			exhausted++
		case 2:
			completed++
		default:
			t.Fatal("concurrent bulk requests split one atomic batch")
		}
	}
	if completed != 1 || exhausted != 1 || bulkDailyUser(t).GetManagerExp()-before.GetManagerExp() != int64(coin.GetRewardManagerExp()+piece.GetRewardManagerExp())*5 {
		t.Fatal("concurrent bulk requests did not settle FREE passes exactly once")
	}
	for _, area := range areas[:2] {
		bulkDailyAssertAreaCount(t, area.GetId(), 5)
	}
	bulkDailyAssertHidden(t, client)
	bulkDailyAssertRetryStreams(t, []string{coin.GetId(), piece.GetId()})
}

func bulkDailyFixture(t *testing.T) ([]*pmaster.Area, map[string]*pmaster.Quest) {
	t.Helper()
	questStartDailyFixture(t)
	areas, quests := questCatalogFixture(t)
	daily := questPermanentDailyAreas(areas)
	if len(daily) != 3 || len(daily[0].GetQuestIds()) < 2 {
		t.Fatal("expected three ordinary daily areas with at least two coin stages")
	}
	return daily, quests
}

func bulkDailyClear(t *testing.T, client papi.QuestClient, number int32, q *pmaster.Quest) {
	t.Helper()
	result := questStartMeasured(t, client, &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 1, IsSkip: true})
	if !result.GetResult().GetCleared() {
		t.Fatalf("bulk fixture failed to clear %s", q.GetId())
	}
}

func bulkDailySaveDeck(t *testing.T, cards []*ptransaction.UserCard) int32 {
	t.Helper()
	bulkDailySetAttributes(t, cards, 1000000)
	positions := make([]*ptransaction.UserDeckPosition, len(cards))
	for i, card := range cards {
		positions[i] = &ptransaction.UserDeckPosition{Position: int32(i + 1), CardId: card.GetCardId()}
	}
	result := profileCall(t, papi.NewDeckClient(testConfig.GrpcConn).Save, &papi.DeckSaveRequest{Name: "一括プレイ検証", UserDeckPositions: positions})
	return result.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber()
}

func bulkDailySetAttributes(t *testing.T, cards []*ptransaction.UserCard, amount int64) {
	t.Helper()
	for _, card := range cards {
		current, err := opuser.FindUserCard(context.Background(), testConfig.TestUserSuid, card.GetCardId())
		if err != nil {
			t.Fatal(err)
		}
		current.Vocal, current.Dance, current.Visual = amount, amount, amount
		current.BaseVocal, current.BaseDance, current.BaseVisual = amount, amount, amount
		if err := opuser.UpsertUserCard(context.Background(), testConfig.TestUserSuid, current); err != nil {
			t.Fatal(err)
		}
	}
}

func bulkDailySeedTickets(t *testing.T, areas []*pmaster.Area) map[string]int64 {
	t.Helper()
	items, err := opmaster.FindAllItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int64, len(areas))
	for _, area := range areas {
		for _, item := range items {
			if item.GetType() == area.GetDailyTicketItemType() {
				setCardTrainingItem(t, item.GetId(), 7)
				counts[item.GetId()] = 7
				break
			}
		}
	}
	if len(counts) != len(areas) {
		t.Fatal("missing ordinary daily ticket definitions")
	}
	return counts
}

func bulkDailyUser(t *testing.T) *ptransaction.User {
	t.Helper()
	user, err := opuser.FindUser(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func bulkDailyAssertAreaCount(t *testing.T, id string, count int32) {
	t.Helper()
	areas, err := opuser.FindAllUserAreas(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, area := range areas {
		if area.GetAreaId() == id {
			if area.GetDailyClearCount() != count {
				t.Fatalf("%s daily count=%d, want %d", id, area.GetDailyClearCount(), count)
			}
			return
		}
	}
	t.Fatalf("missing area state %s", id)
}

func bulkDailyAssertHidden(t *testing.T, client papi.QuestClient) {
	t.Helper()
	if len(profileCall(t, client.Top, &papi.Empty{}).GetBulkDailyInfos()) != 0 || len(profileCall(t, client.ListDaily, &papi.Empty{}).GetBulkDailyInfos()) != 0 {
		t.Fatal("exhausted FREE passes still advertise bulk daily")
	}
}

func bulkDailyAssertRetryStreams(t *testing.T, ids []string) {
	t.Helper()
	ctx, err := mcontext.WithLiveRequest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	parentSeed, started, _ := mcontext.GetLiveRequest(ctx)
	seeds := make(map[string]uint64, len(ids))
	attempts := 0
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		attempts++
		for _, id := range ids {
			variant, err := mcontext.WithLiveRequestVariant(ctx, id)
			if err != nil {
				return nil, err
			}
			seed, at, ok := mcontext.GetLiveRequest(variant)
			if !ok || at != started || seed == parentSeed {
				return nil, errors.New("bulk quest stream lost original request time or reused the parent stream")
			}
			if old, exists := seeds[id]; exists && old != seed {
				return nil, errors.New("transaction retry rerolled a bulk quest")
			}
			seeds[id] = seed
		}
		if attempts == 1 {
			return nil, mongo.CommandError{Code: 112, Message: "injected bulk retry", Labels: []string{"TransientTransactionError"}}
		}
		return nil, nil
	})
	if err != nil || attempts != 2 || seeds[ids[0]] == seeds[ids[1]] {
		t.Fatalf("bulk retry stream isolation failed after %d attempts: %v", attempts, err)
	}
}
