package test

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestQuestStartMainScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	number := questStartDeck(t, 1000000)
	areas, quests := questCatalogFixture(t)
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	q := quests[main[0].GetQuestIds()[0]]
	request := &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 1, IsSkip: true}
	profileReject(t, codes.InvalidArgument, client.Start, &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number})
	profileReject(t, codes.FailedPrecondition, client.Start, &papi.QuestStartRequest{QuestId: main[1].GetQuestIds()[0], DeckNumber: number, PlayCount: 1})
	before, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	balanceBefore, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	cardsBefore := questStartUserDeckCards(t, number)
	// Simultaneous first clears may both simulate, but only one can settle.
	type answer struct {
		result *papi.QuestStartResponse
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
			start := time.Now()
			r, err := client.Start(rpcCtx, request)
			t.Logf("concurrent main Start elapsed=%s result=%v", time.Since(start), status.Code(err))
			answers <- answer{r, err}
		}()
	}
	var result *papi.QuestStartResponse
	var success, rejected int
	for range 2 {
		a := <-answers
		if a.err == nil {
			success++
			result = a.result
		} else if status.Code(a.err) == codes.FailedPrecondition {
			rejected++
		} else {
			t.Fatalf("concurrent Start: %v", a.err)
		}
	}
	if success != 1 || rejected != 1 || !result.GetResult().GetCleared() {
		t.Fatalf("first clear not settled exactly once: success%d rejected%d result%v", success, rejected, result.GetResult().GetCleared())
	}
	after, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if after.GetManagerExp()-before.GetManagerExp() != int64(q.GetRewardManagerExp()) {
		t.Fatal("main manager EXP not granted exactly once")
	}
	stored, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, q.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetDailyClearCount() != 1 || stored.GetLastClearedTime() == 0 || stored.GetHighestScore() != result.GetResult().GetUserInfos()[0].GetUserResult().GetScore() {
		t.Fatal("main clear did not persist result")
	}
	assertQuestStartTrace(t, result)
	if q.GetRewardCardExp() > 0 && len(result.GetReward().GetRewardCards()) != 5 {
		t.Fatal("card EXP projections missing")
	}
	for _, r := range result.GetReward().GetRewardCards() {
		for _, beforeCard := range cardsBefore {
			if beforeCard.GetCardId() == r.GetCardId() {
				afterCard, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, r.GetCardId())
				if err != nil {
					t.Fatal(err)
				}
				if afterCard.GetTotalExp()-beforeCard.GetTotalExp() != int64(r.GetExp()) {
					t.Fatal("persisted card EXP differs from live reward projection")
				}
			}
		}
	}
	var expectedStone int64
	for _, r := range result.GetReward().GetClearRewards() {
		if r.GetResourceType() == penum.ResourceType_ResourceType_Stone && !r.GetIsGift() {
			expectedStone += r.GetAmountI64()
		}
	}
	for _, r := range result.GetReward().GetMusicMasteryRewards() {
		if r.GetResourceType() == penum.ResourceType_ResourceType_Stone {
			expectedStone += r.GetAmount()
		}
	}
	balanceAfter, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if int64(balanceAfter.GetFreeBalance()-balanceBefore.GetFreeBalance()) != expectedStone {
		t.Fatal("clear/mastery diamond rewards were listed without being credited")
	}
	replay := profileCall(t, client.GetLatestClearLiveResult, &papi.QuestGetLatestClearLiveResultRequest{QuestId: q.GetId(), UserId: testConfig.TestUserPublicId})
	if !proto.Equal(result.GetResult(), replay.GetResult()) {
		t.Fatal("stored live replay differs from initial trace")
	}
	replayedBalance, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil || !proto.Equal(replayedBalance, balanceAfter) {
		t.Fatal("reading a replay granted rewards again")
	}
	profileReject(t, codes.FailedPrecondition, client.Start, request)
	get := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if !slices.ContainsFunc(get.GetQuests(), func(x *ptransaction.UserQuest) bool { return x.GetQuestId() == q.GetId() && x.GetLastClearedTime() > 0 }) {
		t.Fatal("User.Get lost completed quest")
	}
	latest := profileCall(t, client.ListLatestClear, &papi.QuestListLatestClearRequest{QuestId: q.GetId()})
	if len(latest.GetClears()) != 0 {
		t.Fatal("private server exposed latest-clear users")
	}
}

func TestQuestStartDailySettlementScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	number := questStartDeck(t, 1000000)
	area, q := questStartDailyFixture(t)
	items, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ticketID string
	for _, item := range items {
		if item.GetType() == area.GetDailyTicketItemType() {
			ticketID = item.GetId()
			break
		}
	}
	if ticketID == "" {
		t.Fatal("daily ticket missing")
	}
	setCardTrainingItem(t, ticketID, 0)
	// FREE play does not require an owned ticket.
	free := questStartMeasured(t, client, &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 1, IsSkip: true})
	if !free.GetResult().GetCleared() || questStartUsablePasses(t, ticketID) != 0 {
		t.Fatal("FREE play consumed an owned ticket or did not clear")
	}
	profileReject(t, codes.FailedPrecondition, client.Start, &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 5})
	questSeedArea(t, area, 1, 5, time.Now())
	request := &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 2, UseDailyTicket: true, IsSkip: true}
	profileReject(t, codes.ResourceExhausted, client.Start, request)
	// Expired tickets never satisfy a paid attempt; live cost is count-scaled.
	if err := opuser.UpsertUserItem(ctx, testConfig.TestUserSuid, &ptransaction.UserItem{ItemId: ticketID, Amount: 100, ExpiredTime: time.Now().Add(-time.Hour).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.ResourceExhausted, client.Start, request)
	setCardTrainingItem(t, ticketID, int64(q.GetTicketAmount())*2+1)
	userBefore, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	// An overflow discovered after simulation/reward application rolls the
	// entire transaction back, including the earlier pass debit and gold writes.
	overflowUser := proto.Clone(userBefore).(*ptransaction.User)
	overflowUser.ManagerExp = math.MaxInt64
	if err := opuser.UpsertUser(ctx, testConfig.TestUserSuid, overflowUser); err != nil {
		t.Fatal(err)
	}
	goldBefore, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	questBeforeOverflow, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, q.GetId())
	if err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.FailedPrecondition, client.Start, request)
	goldAfter, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	questAfterOverflow, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, q.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(goldBefore, goldAfter) || !proto.Equal(questBeforeOverflow, questAfterOverflow) || questStartUsablePasses(t, ticketID) != int64(q.GetTicketAmount())*2+1 {
		t.Fatal("overflow failure did not roll back quest rewards and costs")
	}
	if err := opuser.UpsertUser(ctx, testConfig.TestUserSuid, userBefore); err != nil {
		t.Fatal(err)
	}
	masteryBefore, err := opuser.FindUserMusicMasteries(ctx, testConfig.TestUserSuid, q.GetMusicId())
	if err != nil {
		t.Fatal(err)
	}
	paid := questStartMeasured(t, client, request)
	if !paid.GetResult().GetCleared() || questStartUsablePasses(t, ticketID) != 1 {
		t.Fatalf("paid batch clear=%t usable passes=%d, want true/1", paid.GetResult().GetCleared(), questStartUsablePasses(t, ticketID))
	}
	userAfter, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if userAfter.GetManagerExp()-userBefore.GetManagerExp() != int64(q.GetRewardManagerExp())*2 {
		t.Fatal("multi-play manager EXP not count-scaled")
	}
	states, err := opuser.FindAllUserAreas(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range states {
		if row.GetAreaId() == area.GetId() && row.GetDailyClearCount() != 7 {
			t.Fatal("owned plays must increment daily count beyond five")
		}
	}
	for _, row := range paid.GetCommonResponse().GetUpdatedData().GetCharacterMusics() {
		before := int32(0)
		for _, old := range masteryBefore {
			if old.GetCharacterId() == row.GetCharacterId() {
				before = old.GetMasteryPoint()
				break
			}
		}
		if row.GetMasteryPoint()-before != 2 {
			t.Fatal("mastery not count-scaled per unique character")
		}
	}
	assertQuestStartTrace(t, paid)
	// A later failure must retain clear/best history and consume no owned pass.
	for _, card := range questStartUserDeckCards(t, number) {
		card.Vocal = 0
		card.Dance = 0
		card.Visual = 0
		card.BaseVocal = 0
		card.BaseDance = 0
		card.BaseVisual = 0
		if err := opuser.UpsertUserCard(ctx, testConfig.TestUserSuid, card); err != nil {
			t.Fatal(err)
		}
	}
	setCardTrainingItem(t, ticketID, int64(q.GetTicketAmount())*2)
	beforeQuest, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, q.GetId())
	if err != nil {
		t.Fatal(err)
	}
	failed := questStartMeasured(t, client, request)
	if failed.GetResult().GetCleared() {
		t.Fatal("zero-attribute lineup unexpectedly cleared")
	}
	if questStartUsablePasses(t, ticketID) != int64(q.GetTicketAmount())*2 {
		t.Fatal("failed live consumed passes")
	}
	afterQuest, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, q.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(beforeQuest, afterQuest) {
		t.Fatal("failed repeat changed prior clear/high score")
	}
	if len(failed.GetReward().GetClearRewards())+len(failed.GetReward().GetScoreRankRewards()) != 0 {
		t.Fatal("failure granted rewards")
	}
}

func questStartDeck(t *testing.T, attribute int64) int32 {
	t.Helper()
	ctx := context.Background()
	cards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) < 5 {
		t.Fatal("configured starter user needs five owned cards")
	}
	var positions []*ptransaction.UserDeckPosition
	for i, card := range cards[:5] {
		card.Vocal, card.Dance, card.Visual = attribute, attribute, attribute
		card.BaseVocal, card.BaseDance, card.BaseVisual = attribute, attribute, attribute
		if err := opuser.UpsertUserCard(ctx, testConfig.TestUserSuid, card); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, &ptransaction.UserDeckPosition{Position: int32(i + 1), CardId: card.GetCardId()})
	}
	saved := profileCall(t, papi.NewDeckClient(testConfig.GrpcConn).Save, &papi.DeckSaveRequest{Name: "ライブ検証", UserDeckPositions: positions})
	return saved.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber()
}

func questStartUserDeckCards(t *testing.T, number int32) []*ptransaction.UserCard {
	t.Helper()
	ctx := context.Background()
	positions, err := opuser.FindUserDeckPositions(ctx, testConfig.TestUserSuid, number)
	if err != nil {
		t.Fatal(err)
	}
	var cards []*ptransaction.UserCard
	for _, p := range positions {
		card, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, p.GetCardId())
		if err != nil {
			t.Fatal(err)
		}
		cards = append(cards, card)
	}
	return cards
}

func questStartDailyFixture(t *testing.T) (*pmaster.Area, *pmaster.Quest) {
	t.Helper()
	areas, quests := questCatalogFixture(t)
	daily := questPermanentDailyAreas(areas)
	area := daily[0]
	q := quests[area.GetQuestIds()[0]]
	condition, err := opmaster.FindCondition(context.Background(), q.GetUnlockConditionId())
	if err != nil {
		t.Fatal(err)
	}
	var required int32
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	for _, setting := range condition.GetSettings() {
		for _, a := range main {
			if a.GetId() == setting.GetClearArea().GetAreaId() {
				required = max(required, a.GetOrder())
			}
		}
	}
	for _, a := range main {
		if a.GetOrder() <= required {
			questSeedFullArea(t, a)
		}
	}
	maximum, err := opmaster.FindMaxManagerLevel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	questSetManagerLevel(t, maximum.GetLevel())
	return area, q
}

func assertQuestStartTrace(t *testing.T, result *papi.QuestStartResponse) {
	t.Helper()
	if result.GetCommonResponse() == nil || result.GetResult().GetDetailLogId() == "" || len(result.GetResult().GetCharts()) == 0 {
		t.Fatal("live result omitted trace/history/common data")
	}
	user := result.GetResult().GetUserInfos()[0]
	if len(user.GetUserDeck().GetCards()) != 5 {
		t.Fatal("live requires five own cards")
	}
	var sum int64
	for _, card := range user.GetUserResult().GetCardResults() {
		sum += card.GetTotalScore()
	}
	if sum != user.GetUserResult().GetScore() {
		t.Fatal("card scores do not sum to user score")
	}
}

func questStartUsablePasses(t *testing.T, id string) int64 {
	t.Helper()
	rows, err := opuser.FindUserItems(context.Background(), testConfig.TestUserSuid, id)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, row := range rows {
		if row.GetExpiredTime() == 0 || row.GetExpiredTime() > time.Now().UnixMilli() {
			total += row.GetAmount()
		}
	}
	return total
}

func questStartMeasured(t *testing.T, client papi.QuestClient, req *papi.QuestStartRequest) *papi.QuestStartResponse {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	start := time.Now()
	response, err := client.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Logf("Start quest=%s count=%d clear=%t elapsed=%s protobufBytes=%d", req.GetQuestId(), req.GetPlayCount(), response.GetResult().GetCleared(), time.Since(start), proto.Size(response))
	return response
}
