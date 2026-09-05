package test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGuildScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	client := papi.NewGuildClient(testConfig.GrpcConn)
	masters, err := opmaster.FindAllGuilds(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(masters) != 8 {
		t.Fatalf("Guild master count = %d, want 8", len(masters))
	}

	list := hierarchyGuildCall(t, func(ctx context.Context) (*papi.GuildListResponse, error) {
		return client.List(ctx, &papi.Empty{})
	})
	requireHierarchyGuildCommonResponse(t, list.GetCommonResponse())
	if len(list.GetGuilds()) != len(masters) {
		t.Fatalf("Guild.List count = %d, want current master count %d", len(list.GetGuilds()), len(masters))
	}
	for i, master := range masters {
		if list.GetGuilds()[i].GetId() != master.GetId() {
			t.Fatalf("Guild.List guild[%d] = %q, want master guild %q", i, list.GetGuilds()[i].GetId(), master.GetId())
		}
	}

	top := hierarchyGuildCall(t, func(ctx context.Context) (*papi.GuildTopResponse, error) {
		return client.Top(ctx, &papi.Empty{})
	})
	requireHierarchyGuildCommonResponse(t, top.GetCommonResponse())
	if top.GetRewardGuildMedalAmount() != 0 || top.GetGvgSeasonInfo() != nil || top.GetExchangeBoothId() != "" {
		t.Fatalf("Guild.Top returned unsupported multiplayer state: %v", top)
	}
	assertNoGuildCheckInMutation(t, top)

	joinMaster := masters[len(masters)/2]
	joinStarted := time.Now().UnixMilli()
	join := hierarchyGuildCall(t, func(ctx context.Context) (*papi.GuildJoinResponse, error) {
		return client.Join(ctx, &papi.GuildJoinRequest{GuildId: joinMaster.GetId()})
	})
	joinFinished := time.Now().UnixMilli()
	requireHierarchyGuildCommonResponse(t, join.GetCommonResponse())
	updatedUser := join.GetCommonResponse().GetUpdatedData().GetUser()
	assertGuildMembership(t, updatedUser, joinMaster.GetId(), joinStarted, joinFinished)

	storedUser, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertGuildMembership(t, storedUser, joinMaster.GetId(), joinStarted, joinFinished)

	userGet := hierarchyGuildCall(t, func(ctx context.Context) (*papi.UserGetResponse, error) {
		return papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	})
	assertGuildMembership(t, userGet.GetUser(), joinMaster.GetId(), joinStarted, joinFinished)
	afterJoinTop := hierarchyGuildCall(t, func(ctx context.Context) (*papi.GuildTopResponse, error) {
		return client.Top(ctx, &papi.Empty{})
	})
	assertNoGuildCheckInMutation(t, afterJoinTop)

	otherMaster := masters[0]
	if otherMaster.GetId() == joinMaster.GetId() {
		otherMaster = masters[1]
	}
	switchStarted := time.Now().UnixMilli()
	hierarchyGuildCall(t, func(ctx context.Context) (*papi.GuildJoinResponse, error) {
		return client.Join(ctx, &papi.GuildJoinRequest{
			GuildId:       otherMaster.GetId(),
			IsRecommended: true,
		})
	})
	switchFinished := time.Now().UnixMilli()
	storedUser, err = opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertGuildMembership(t, storedUser, otherMaster.GetId(), switchStarted, switchFinished)

	storedUser.LastGuildCheckTime = time.Now().Add(-24 * time.Hour).UnixMilli()
	if err := opuser.UpsertUser(dbCtx, testConfig.TestUserSuid, storedUser); err != nil {
		t.Fatal(err)
	}
	medalsBefore := guildMedalAmount(t, dbCtx)
	totalBefore, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	checkInStarted := time.Now().UnixMilli()
	checkIn := hierarchyGuildCall(t, func(ctx context.Context) (*papi.GuildTopResponse, error) {
		return client.Top(ctx, &papi.Empty{})
	})
	checkInFinished := time.Now().UnixMilli()
	requireHierarchyGuildCommonResponse(t, checkIn.GetCommonResponse())
	checkInReward := int64(checkIn.GetRewardGuildMedalAmount())
	if checkInReward <= 0 {
		t.Fatalf("first Guild.Top of the JST day rewarded %d medals, want a positive current reward", checkInReward)
	}
	assertGuildMembership(
		t,
		checkIn.GetCommonResponse().GetUpdatedData().GetUser(),
		otherMaster.GetId(),
		checkInStarted,
		checkInFinished,
	)
	assertGuildCheckInState(t, dbCtx, checkIn, medalsBefore+checkInReward, totalBefore.GetGuildCheckInCount()+1)

	storedAfterCheckIn, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	checkInTime := storedAfterCheckIn.GetLastGuildCheckTime()
	secondTop := hierarchyGuildCall(t, func(ctx context.Context) (*papi.GuildTopResponse, error) {
		return client.Top(ctx, &papi.Empty{})
	})
	requireHierarchyGuildCommonResponse(t, secondTop.GetCommonResponse())
	if secondTop.GetRewardGuildMedalAmount() != 0 {
		t.Fatalf("repeated Guild.Top on the same JST day rewarded %d medals, want 0", secondTop.GetRewardGuildMedalAmount())
	}
	assertNoGuildCheckInMutation(t, secondTop)
	if got := guildMedalAmount(t, dbCtx); got != medalsBefore+checkInReward {
		t.Fatalf("persisted guild medals after repeated Guild.Top = %d, want %d", got, medalsBefore+checkInReward)
	}
	storedAfterSecondTop, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if storedAfterSecondTop.GetLastGuildCheckTime() != checkInTime {
		t.Fatalf("repeated Guild.Top changed lastGuildCheckTime from %d to %d", checkInTime, storedAfterSecondTop.GetLastGuildCheckTime())
	}
}

func TestHierarchyScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	client := papi.NewHierarchyClient(testConfig.GrpcConn)
	grades, err := opmaster.FindAllHierarchyDetailGrades(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	rewards, err := opmaster.FindAllHierarchyRewards(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	divisionRewards, err := opmaster.FindAllHierarchyDivisionRewards(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(grades) == 0 || len(rewards) == 0 || len(divisionRewards) == 0 {
		t.Fatal("Hierarchy scenario requires detail grades and both reward catalogs")
	}

	strongest := strongestHierarchyGradeForTest(grades)
	const variablePoint int64 = 37
	totalPoint := strongest.GetRequiredHierarchyPoint() + variablePoint
	hierarchy, err := opuser.FindUserHierarchy(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	hierarchy.BestDetailGradeId = strongest.GetId()
	hierarchy.CurrentFixedPoint = strongest.GetRequiredHierarchyPoint()
	hierarchy.CurrentVariablePoint = variablePoint
	hierarchy.BestTotalPoint = totalPoint
	hierarchy.CurrentPointUpdateDatetime = time.Now().UnixMilli()
	if err := opuser.UpsertUserHierarchy(dbCtx, testConfig.TestUserSuid, hierarchy); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserHierarchyNotiInfo(dbCtx, testConfig.TestUserSuid, &papi.HierarchyNotiInfo{
		CurrentDetailGradeId: "stale-grade",
		CurrentRank:          99,
	}); err != nil {
		t.Fatal(err)
	}

	home := hierarchyGuildCall(t, func(ctx context.Context) (*papi.HomeEnterResponse, error) {
		return papi.NewHomeClient(testConfig.GrpcConn).Enter(ctx, &papi.HomeEnterRequest{})
	})
	assertHierarchyHomeNotification(t, home.GetHierarchyNotiInfo(), strongest.GetId(), rewards, divisionRewards)

	top := hierarchyGuildCall(t, func(ctx context.Context) (*papi.HierarchyTopResponse, error) {
		return client.Top(ctx, &papi.Empty{})
	})
	requireHierarchyGuildCommonResponse(t, top.GetCommonResponse())
	if info := top.GetUserHierarchyInfo(); info == nil || info.GetCurrentRank() != 1 ||
		info.GetCurrentPoint() != totalPoint || info.GetCurrentDetailGradeId() != strongest.GetId() {
		t.Fatalf("Hierarchy.Top returned inconsistent self state: %v", top.GetUserHierarchyInfo())
	}
	assertHierarchyRewardCatalog(t, top.GetRewardInfos(), rewards)
	assertHierarchyDivisionRewardCatalog(t, top.GetDivisionRewardInfos(), divisionRewards)

	ranking := hierarchyGuildCall(t, func(ctx context.Context) (*papi.HierarchyRankingResponse, error) {
		return client.ListRanking(ctx, &papi.Empty{})
	})
	requireHierarchyGuildCommonResponse(t, ranking.GetCommonResponse())
	assertHierarchySelfRanking(t, dbCtx, ranking.GetRankUserInfos(), strongest.GetId(), totalPoint)

	reward := selectAutomaticStoneHierarchyReward(t, rewards)
	balanceBefore, err := opuser.FindUserBalance(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	receive := hierarchyGuildCall(t, func(ctx context.Context) (*papi.HierarchyRewardsReceiveResponse, error) {
		return client.ReceiveRewards(ctx, &papi.HierarchyRewardsReceiveRequest{
			HierarchyRewardIds: []string{reward.GetId()},
		})
	})
	requireHierarchyGuildCommonResponse(t, receive.GetCommonResponse())
	assertHierarchyStoneReward(t, receive.GetResults(), balanceBefore.GetFreeBalance(), automaticStoneAmount(t, reward.GetRewardId()))
	assertReceivedHierarchyReward(t, dbCtx, reward.GetId(), false)
	if updated := receive.GetCommonResponse().GetUpdatedData().GetHierarchy(); updated == nil || !slices.Contains(updated.GetReceivedRewardIds(), reward.GetId()) {
		t.Fatalf("Hierarchy.ReceiveRewards omitted updated hierarchy state: %v", receive)
	}
	assertDuplicateHierarchyRewardRejected(t, func(ctx context.Context) error {
		_, err := client.ReceiveRewards(ctx, &papi.HierarchyRewardsReceiveRequest{
			HierarchyRewardIds: []string{reward.GetId()},
		})
		return err
	})

	divisionReward := selectAutomaticStoneHierarchyDivisionReward(t, divisionRewards)
	balanceBefore, err = opuser.FindUserBalance(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	receiveDivision := hierarchyGuildCall(t, func(ctx context.Context) (*papi.HierarchyDivisionRewardsReceiveResponse, error) {
		return client.ReceiveDivisionRewards(ctx, &papi.HierarchyDivisionRewardsReceiveRequest{
			HierarchyDivisionRewardIds: []string{divisionReward.GetId()},
		})
	})
	requireHierarchyGuildCommonResponse(t, receiveDivision.GetCommonResponse())
	assertHierarchyStoneReward(t, receiveDivision.GetResults(), balanceBefore.GetFreeBalance(), automaticStoneAmount(t, divisionReward.GetRewardId()))
	assertReceivedHierarchyReward(t, dbCtx, divisionReward.GetId(), true)
	if updated := receiveDivision.GetCommonResponse().GetUpdatedData().GetHierarchy(); updated == nil || !slices.Contains(updated.GetReceivedDivisionRewardIds(), divisionReward.GetId()) {
		t.Fatalf("Hierarchy.ReceiveDivisionRewards omitted updated hierarchy state: %v", receiveDivision)
	}
	assertDuplicateHierarchyRewardRejected(t, func(ctx context.Context) error {
		_, err := client.ReceiveDivisionRewards(ctx, &papi.HierarchyDivisionRewardsReceiveRequest{
			HierarchyDivisionRewardIds: []string{divisionReward.GetId()},
		})
		return err
	})

	finalTop := hierarchyGuildCall(t, func(ctx context.Context) (*papi.HierarchyTopResponse, error) {
		return client.Top(ctx, &papi.Empty{})
	})
	if info := findHierarchyRewardInfo(finalTop.GetRewardInfos(), reward.GetId()); info == nil || !info.GetHaveReceived() {
		t.Fatalf("Hierarchy.Top did not report received reward %q: %v", reward.GetId(), finalTop)
	}
	if info := findHierarchyDivisionRewardInfo(finalTop.GetDivisionRewardInfos(), divisionReward.GetId()); info == nil || !info.GetHaveReceived() {
		t.Fatalf("Hierarchy.Top did not report received division reward %q: %v", divisionReward.GetId(), finalTop)
	}

	userGet := hierarchyGuildCall(t, func(ctx context.Context) (*papi.UserGetResponse, error) {
		return papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	})
	if userGet.GetHierarchy() == nil ||
		!slices.Contains(userGet.GetHierarchy().GetReceivedRewardIds(), reward.GetId()) ||
		!slices.Contains(userGet.GetHierarchy().GetReceivedDivisionRewardIds(), divisionReward.GetId()) {
		t.Fatalf("User.Get omitted persisted Hierarchy reward state: %v", userGet.GetHierarchy())
	}
}

func assertHierarchyHomeNotification(
	t *testing.T,
	info *papi.HierarchyNotiInfo,
	detailGradeID string,
	rewards []*pmaster.HierarchyReward,
	divisionRewards []*pmaster.HierarchyDivisionReward,
) {
	t.Helper()
	if info == nil || info.GetCurrentRank() != 1 || info.GetCurrentDetailGradeId() != detailGradeID {
		t.Fatalf("Home.Enter returned stale hierarchy notification state: %v", info)
	}
	wantRewardIDs := make([]string, 0, len(rewards))
	for _, reward := range rewards {
		wantRewardIDs = append(wantRewardIDs, reward.GetId())
	}
	wantDivisionRewardIDs := make([]string, 0, len(divisionRewards))
	for _, reward := range divisionRewards {
		wantDivisionRewardIDs = append(wantDivisionRewardIDs, reward.GetId())
	}
	if !slices.Equal(info.GetRewardIds(), wantRewardIDs) ||
		!slices.Equal(info.GetDivisionRewardIds(), wantDivisionRewardIDs) {
		t.Fatalf("Home.Enter hierarchy notification rewards differ from current master catalogs: %v", info)
	}
}

func hierarchyGuildCall[T any](t *testing.T, call func(context.Context) (T, error)) T {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		var zero T
		t.Fatal(err)
		return zero
	}
	defer cancel()
	response, err := call(ctx)
	if err != nil {
		var zero T
		t.Fatal(err)
		return zero
	}
	return response
}

func requireHierarchyGuildCommonResponse(t *testing.T, response *papi.CommonResponse) {
	t.Helper()
	if response == nil || response.GetUpdatedData() == nil || response.GetDeletedData() == nil {
		t.Fatalf("Hierarchy/Guild API returned an incomplete common response: %v", response)
	}
}

func assertGuildMembership(t *testing.T, user *ptransaction.User, guildID string, from, through int64) {
	t.Helper()
	if user == nil || user.GetGuildId() != guildID ||
		user.GetLastGuildCheckTime() < from || user.GetLastGuildCheckTime() > through {
		t.Fatalf("guild membership = %v, want guild %q and check time in [%d,%d]", user, guildID, from, through)
	}
}

func assertNoGuildCheckInMutation(t *testing.T, response *papi.GuildTopResponse) {
	t.Helper()
	requireHierarchyGuildCommonResponse(t, response.GetCommonResponse())
	if response.GetRewardGuildMedalAmount() != 0 {
		t.Fatalf("Guild.Top rewarded %d medals when no check-in was due", response.GetRewardGuildMedalAmount())
	}
	if updated := response.GetCommonResponse().GetUpdatedData(); updated.GetUser() != nil ||
		updated.GetTotalCount() != nil || len(updated.GetPoints()) != 0 || len(updated.GetMissions()) != 0 {
		t.Fatalf("Guild.Top unexpectedly updated check-in data: %v", updated)
	}
}

func assertGuildCheckInState(
	t *testing.T,
	ctx context.Context,
	response *papi.GuildTopResponse,
	wantMedals int64,
	wantCheckInCount int64,
) {
	t.Helper()
	updated := response.GetCommonResponse().GetUpdatedData()
	var updatedMedals *ptransaction.UserPoint
	for _, point := range updated.GetPoints() {
		if point.GetPointType() == penum.PointType_PointType_GuildMedal {
			updatedMedals = point
			break
		}
	}
	if updatedMedals == nil || updatedMedals.GetAmount() != wantMedals {
		t.Fatalf("Guild.Top updated guild medals = %v, want %d", updatedMedals, wantMedals)
	}
	if updated.GetTotalCount() == nil || updated.GetTotalCount().GetGuildCheckInCount() != wantCheckInCount {
		t.Fatalf("Guild.Top updated check-in total = %v, want %d", updated.GetTotalCount(), wantCheckInCount)
	}
	if got := guildMedalAmount(t, ctx); got != wantMedals {
		t.Fatalf("persisted guild medals = %d, want %d", got, wantMedals)
	}
	storedTotal, err := opuser.FindUserTotalCount(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if storedTotal.GetGuildCheckInCount() != wantCheckInCount {
		t.Fatalf("persisted guild check-in count = %d, want %d", storedTotal.GetGuildCheckInCount(), wantCheckInCount)
	}
}

func guildMedalAmount(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	point, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_GuildMedal)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return point.GetAmount()
}

func strongestHierarchyGradeForTest(grades []*pmaster.HierarchyDetailGrade) *pmaster.HierarchyDetailGrade {
	strongest := grades[0]
	for _, grade := range grades[1:] {
		if grade.GetRequiredHierarchyPoint() > strongest.GetRequiredHierarchyPoint() {
			strongest = grade
		}
	}
	return strongest
}

func assertHierarchyRewardCatalog(t *testing.T, infos []*papi.HierarchyRewardInfo, masters []*pmaster.HierarchyReward) {
	t.Helper()
	if len(infos) != len(masters) {
		t.Fatalf("Hierarchy.Top reward count = %d, want current master count %d", len(infos), len(masters))
	}
	for _, master := range masters {
		info := findHierarchyRewardInfo(infos, master.GetId())
		if info == nil || !info.GetCanReceive() || info.GetHaveReceived() {
			t.Fatalf("Hierarchy.Top reward %q is inconsistent at the highest grade: %v", master.GetId(), info)
		}
	}
}

func assertHierarchyDivisionRewardCatalog(
	t *testing.T,
	infos []*papi.HierarchyDivisionRewardInfo,
	masters []*pmaster.HierarchyDivisionReward,
) {
	t.Helper()
	if len(infos) != len(masters) {
		t.Fatalf("Hierarchy.Top division reward count = %d, want current master count %d", len(infos), len(masters))
	}
	for _, master := range masters {
		info := findHierarchyDivisionRewardInfo(infos, master.GetId())
		if info == nil || !info.GetCanReceive() || info.GetHaveReceived() {
			t.Fatalf("Hierarchy.Top division reward %q is inconsistent at the highest grade: %v", master.GetId(), info)
		}
		if info.GetFirstUserInfo() == nil || info.GetFirstUserInfo().GetUserId() != testConfig.TestUserPublicId {
			t.Fatalf("Hierarchy.Top division reward %q did not identify the private-server user as first: %v", master.GetId(), info)
		}
	}
}

func assertHierarchySelfRanking(
	t *testing.T,
	ctx context.Context,
	infos []*papi.HierarchyRankUserInfo,
	detailGradeID string,
	point int64,
) {
	t.Helper()
	if len(infos) != 1 {
		t.Fatalf("Hierarchy.ListRanking returned %d users, want only self", len(infos))
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	info := infos[0]
	if info.GetUserId() != testConfig.TestUserPublicId || info.GetCurrentRank() != 1 ||
		info.GetManagerName() != user.GetName() || info.GetManagerLevel() != user.GetManagerLevel() ||
		info.GetCurrentDetailGradeId() != detailGradeID || info.GetCurrentPoint() != point ||
		info.GetEmblemId() != user.GetEmblemId() {
		t.Fatalf("Hierarchy.ListRanking self row is inconsistent with current user state: %v", info)
	}

	decks, err := opuser.FindAllUserDecks(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(decks) == 0 {
		t.Fatal("configured test user has no deck")
	}
	slices.SortFunc(decks, func(a, b *ptransaction.UserDeck) int {
		return int(a.GetNumber() - b.GetNumber())
	})
	if info.GetDeckName() != decks[0].GetName() {
		t.Fatalf("Hierarchy.ListRanking deck name = %q, want %q", info.GetDeckName(), decks[0].GetName())
	}
	positions, err := opuser.FindUserDeckPositions(ctx, testConfig.TestUserSuid, decks[0].GetNumber())
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(positions, func(a, b *ptransaction.UserDeckPosition) int {
		return int(a.GetPosition() - b.GetPosition())
	})
	if len(info.GetCardInfos()) != len(positions) {
		t.Fatalf("Hierarchy.ListRanking card count = %d, want deck position count %d", len(info.GetCardInfos()), len(positions))
	}
	for i, position := range positions {
		card, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, position.GetCardId())
		if err != nil {
			t.Fatal(err)
		}
		got := info.GetCardInfos()[i]
		if got.GetPosition() != position.GetPosition() || got.GetCardId() != card.GetCardId() ||
			got.GetLevel() != card.GetLevel() || got.GetRarity() != card.GetRarity() ||
			got.GetEvolutionLevel() != card.GetEvolutionLevel() {
			t.Fatalf("Hierarchy.ListRanking card[%d] = %v, want position=%v card=%v", i, got, position, card)
		}
	}
}

func selectAutomaticStoneHierarchyReward(t *testing.T, rewards []*pmaster.HierarchyReward) *pmaster.HierarchyReward {
	t.Helper()
	for _, reward := range rewards {
		if _, ok := parseAutomaticStoneAmount(reward.GetRewardId()); ok {
			return reward
		}
	}
	t.Fatal("HierarchyReward master has no automatic stone reward")
	return nil
}

func selectAutomaticStoneHierarchyDivisionReward(
	t *testing.T,
	rewards []*pmaster.HierarchyDivisionReward,
) *pmaster.HierarchyDivisionReward {
	t.Helper()
	for _, reward := range rewards {
		if _, ok := parseAutomaticStoneAmount(reward.GetRewardId()); ok {
			return reward
		}
	}
	t.Fatal("HierarchyDivisionReward master has no automatic stone reward")
	return nil
}

func automaticStoneAmount(t *testing.T, rewardID string) int64 {
	t.Helper()
	amount, ok := parseAutomaticStoneAmount(rewardID)
	if !ok {
		t.Fatalf("reward %q is not an automatic stone reward", rewardID)
	}
	return amount
}

func parseAutomaticStoneAmount(rewardID string) (int64, bool) {
	parts := strings.Split(rewardID, ";")
	if len(parts) != 4 || parts[0] != "rd_auto" || parts[1] != "stone" || parts[2] != "" {
		return 0, false
	}
	amount, err := strconv.ParseInt(parts[3], 10, 64)
	return amount, err == nil && amount > 0
}

func assertHierarchyStoneReward(t *testing.T, results []*ptransaction.RewardResult, before int32, amount int64) {
	t.Helper()
	for _, result := range results {
		if result.GetResourceType() != penum.ResourceType_ResourceType_Stone {
			continue
		}
		if result.GetBeforeAmount() != int64(before) || result.GetAfterAmount() != int64(before)+amount {
			t.Fatalf("Hierarchy stone result = %v, want %d -> %d", result, before, int64(before)+amount)
		}
		balance, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
		if err != nil {
			t.Fatal(err)
		}
		if int64(balance.GetFreeBalance()) != result.GetAfterAmount() {
			t.Fatalf("persisted free balance = %d, want reward result amount %d", balance.GetFreeBalance(), result.GetAfterAmount())
		}
		return
	}
	t.Fatalf("Hierarchy reward results contain no stone result: %v", results)
}

func assertReceivedHierarchyReward(t *testing.T, ctx context.Context, id string, division bool) {
	t.Helper()
	hierarchy, err := opuser.FindUserHierarchy(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ids := hierarchy.GetReceivedRewardIds()
	if division {
		ids = hierarchy.GetReceivedDivisionRewardIds()
	}
	if !slices.Contains(ids, id) {
		t.Fatalf("persisted Hierarchy state does not contain received reward %q: %v", id, hierarchy)
	}
}

func assertDuplicateHierarchyRewardRejected(t *testing.T, call func(context.Context) error) {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	err = call(ctx)
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("duplicate Hierarchy reward error = %v, want FailedPrecondition", err)
	}
}

func findHierarchyRewardInfo(infos []*papi.HierarchyRewardInfo, id string) *papi.HierarchyRewardInfo {
	for _, info := range infos {
		if info.GetHierarchyRewardId() == id {
			return info
		}
	}
	return nil
}

func findHierarchyDivisionRewardInfo(
	infos []*papi.HierarchyDivisionRewardInfo,
	id string,
) *papi.HierarchyDivisionRewardInfo {
	for _, info := range infos {
		if info.GetHierarchyDivisionRewardId() == id {
			return info
		}
	}
	return nil
}
