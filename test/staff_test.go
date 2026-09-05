package test

import (
	"context"
	"errors"
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

const staffLimitBreakItemID = "item-staff-limit-break"

type staffScenarioCatalog struct {
	parameterType penum.ParameterType
	trainings     []*pmaster.StaffTraining
	levels        map[int32]*pmaster.StaffLevel
	rankZero      *pmaster.StaffLevelLimitBreakRank
	rankOne       *pmaster.StaffLevelLimitBreakRank
}

func TestStaffScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	client := papi.NewStaffClient(testConfig.GrpcConn)
	catalog := loadStaffScenarioCatalog(t, dbCtx)
	levelLimit := catalog.rankZero.GetLevelLimit()
	initialTargetLevel := int32(3)
	initialTrainCount := int64(initialTargetLevel - 1)
	initialTrainCost := staffMedalCost(t, catalog.levels, 1, initialTargetLevel)
	totalTrainCost := staffMedalCost(t, catalog.levels, 1, levelLimit)
	secondTrainCost := staffMedalCost(t, catalog.levels, 1, 2)

	rejectedCtx, rejectedCancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Train(rejectedCtx, &papi.StaffTrainRequest{
		ParameterType: catalog.parameterType,
		TrainCount:    1,
	})
	rejectedCancel()
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("Staff.Train without guild medals returned %v, want %v", err, codes.ResourceExhausted)
	}
	assertStoredStaffState(t, dbCtx, catalog.parameterType, 1, 0)

	const resourceBuffer int64 = 12345
	seedGuildMedals := totalTrainCost + resourceBuffer
	seedGold := catalog.rankOne.GetConsumptionGoldAmount() + resourceBuffer
	seedLimitBreakItems := catalog.rankOne.GetConsumptionItemAmount() + 5
	if err := opuser.UpsertUserPoint(dbCtx, testConfig.TestUserSuid, &ptransaction.UserPoint{
		PointType: penum.PointType_PointType_GuildMedal,
		Amount:    seedGuildMedals,
	}); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserPoint(dbCtx, testConfig.TestUserSuid, &ptransaction.UserPoint{
		PointType: penum.PointType_PointType_Gold,
		Amount:    seedGold,
	}); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: staffLimitBreakItemID,
		Amount: seedLimitBreakItems,
	}); err != nil {
		t.Fatal(err)
	}
	totalCount, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	totalCount.StaffTrainCount = 0
	if err := opuser.UpsertUserTotalCount(dbCtx, testConfig.TestUserSuid, totalCount); err != nil {
		t.Fatal(err)
	}
	earnedGoldBefore := totalCount.GetPointGoldAmount()
	balanceBefore, err := opuser.FindUserBalance(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	freeBalanceBefore, paidBalanceBefore := balanceBefore.GetFreeBalance(), balanceBefore.GetPaidBalance()

	firstTrain := staffCall(t, func(ctx context.Context) (*papi.StaffTrainResponse, error) {
		return client.Train(ctx, &papi.StaffTrainRequest{
			ParameterType: catalog.parameterType,
			TrainCount:    initialTrainCount,
		})
	})
	firstUpdated := requireStaffCommonResponse(t, "Staff.Train", firstTrain.GetCommonResponse())
	assertStaff(t, findStaff(firstUpdated.GetStaffs(), catalog.parameterType), initialTargetLevel, 0)
	if point := findPoint(firstUpdated.GetPoints(), penum.PointType_PointType_GuildMedal); point == nil ||
		point.GetAmount() != seedGuildMedals-initialTrainCost {
		t.Fatalf("first Staff.Train updated guild medals = %v, want %d", point, seedGuildMedals-initialTrainCost)
	}
	if firstUpdated.GetTotalCount().GetStaffTrainCount() != initialTrainCount {
		t.Fatalf("first Staff.Train total count = %d, want %d",
			firstUpdated.GetTotalCount().GetStaffTrainCount(), initialTrainCount)
	}
	assertNoStaffCardMutation(t, "Staff.Train", firstUpdated)
	assertStoredStaffState(t, dbCtx, catalog.parameterType, initialTargetLevel, 0)
	assertStoredStaffResources(t, dbCtx, seedGuildMedals-initialTrainCost, seedGold, seedLimitBreakItems)
	assertStoredStaffTrainCount(t, dbCtx, initialTrainCount)

	remainingTrainCount := int64(levelLimit - initialTargetLevel)
	toCap := staffCall(t, func(ctx context.Context) (*papi.StaffTrainResponse, error) {
		return client.Train(ctx, &papi.StaffTrainRequest{
			ParameterType: catalog.parameterType,
			TrainCount:    remainingTrainCount,
		})
	})
	toCapUpdated := requireStaffCommonResponse(t, "Staff.Train to cap", toCap.GetCommonResponse())
	assertStaff(t, findStaff(toCapUpdated.GetStaffs(), catalog.parameterType), levelLimit, 0)
	wantHighWater := int64(levelLimit - 1)
	if toCapUpdated.GetTotalCount().GetStaffTrainCount() != wantHighWater {
		t.Fatalf("Staff.Train to cap total count = %d, want %d",
			toCapUpdated.GetTotalCount().GetStaffTrainCount(), wantHighWater)
	}
	assertNoStaffCardMutation(t, "Staff.Train to cap", toCapUpdated)
	assertStoredStaffState(t, dbCtx, catalog.parameterType, levelLimit, 0)
	assertStoredStaffResources(t, dbCtx, seedGuildMedals-totalTrainCost, seedGold, seedLimitBreakItems)
	assertStoredStaffTrainCount(t, dbCtx, wantHighWater)

	medalsBeforeRejectedTrain := staffPointAmount(t, dbCtx, penum.PointType_PointType_GuildMedal)
	goldBeforeRejectedTrain := staffPointAmount(t, dbCtx, penum.PointType_PointType_Gold)
	itemsBeforeRejectedTrain := staffItemAmount(t, dbCtx, staffLimitBreakItemID)
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Train(ctx, &papi.StaffTrainRequest{
		ParameterType: catalog.parameterType,
		TrainCount:    1,
	})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("Staff.Train beyond rank cap returned %v, want %v", err, codes.FailedPrecondition)
	}
	assertStoredStaffState(t, dbCtx, catalog.parameterType, levelLimit, 0)
	assertStoredStaffResources(t, dbCtx, medalsBeforeRejectedTrain, goldBeforeRejectedTrain, itemsBeforeRejectedTrain)
	assertStoredStaffTrainCount(t, dbCtx, wantHighWater)

	satisfyStaffLevelConditionForTest(t, dbCtx, catalog.levels[levelLimit+1])
	limitBreak := staffCall(t, func(ctx context.Context) (*papi.StaffLevelLimitBreakResponse, error) {
		return client.LevelLimitBreak(ctx, &papi.StaffLevelLimitBreakRequest{
			ParameterType: catalog.parameterType,
		})
	})
	limitBreakUpdated := requireStaffCommonResponse(t, "Staff.LevelLimitBreak", limitBreak.GetCommonResponse())
	assertStaff(t, findStaff(limitBreakUpdated.GetStaffs(), catalog.parameterType), levelLimit, catalog.rankOne.GetRank())
	wantGoldAfterLimitBreak := seedGold - catalog.rankOne.GetConsumptionGoldAmount()
	wantItemsAfterLimitBreak := seedLimitBreakItems - catalog.rankOne.GetConsumptionItemAmount()
	if gold := findPoint(limitBreakUpdated.GetPoints(), penum.PointType_PointType_Gold); gold == nil ||
		gold.GetAmount() != wantGoldAfterLimitBreak {
		t.Fatalf("Staff.LevelLimitBreak updated gold = %v, want %d", gold, wantGoldAfterLimitBreak)
	}
	if amount := findItemAmount(limitBreakUpdated.GetItems(), staffLimitBreakItemID); amount != wantItemsAfterLimitBreak {
		t.Fatalf("Staff.LevelLimitBreak updated limit-break items = %d, want %d", amount, wantItemsAfterLimitBreak)
	}
	assertNoStaffCardMutation(t, "Staff.LevelLimitBreak", limitBreakUpdated)
	assertStoredStaffState(t, dbCtx, catalog.parameterType, levelLimit, catalog.rankOne.GetRank())
	assertStoredStaffResources(t, dbCtx, seedGuildMedals-totalTrainCost, wantGoldAfterLimitBreak, wantItemsAfterLimitBreak)
	assertStoredStaffTrainCount(t, dbCtx, wantHighWater)

	afterLimitBreak := staffCall(t, func(ctx context.Context) (*papi.UserGetResponse, error) {
		return papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	})
	assertStaff(t, findStaff(afterLimitBreak.GetStaffs(), catalog.parameterType), levelLimit, catalog.rankOne.GetRank())
	if got := findPoint(afterLimitBreak.GetPoints(), penum.PointType_PointType_Gold).GetAmount(); got != wantGoldAfterLimitBreak {
		t.Fatalf("User.Get gold after Staff.LevelLimitBreak = %d, want %d", got, wantGoldAfterLimitBreak)
	}
	if got := findItemAmount(afterLimitBreak.GetItems(), staffLimitBreakItemID); got != wantItemsAfterLimitBreak {
		t.Fatalf("User.Get limit-break items after Staff.LevelLimitBreak = %d, want %d", got, wantItemsAfterLimitBreak)
	}

	firstReset := staffCall(t, func(ctx context.Context) (*papi.StaffResetResponse, error) {
		return client.Reset(ctx, &papi.Empty{})
	})
	assertStaffResetRewards(t, firstReset.GetRewardResults(), map[staffRewardKey]int64{
		{resourceType: penum.ResourceType_ResourceType_Item, resourceID: staffLimitBreakItemID}: catalog.rankOne.GetConsumptionItemAmount(),
		{resourceType: penum.ResourceType_ResourceType_Gold}:                                    catalog.rankOne.GetConsumptionGoldAmount(),
		{resourceType: penum.ResourceType_ResourceType_GuildMedal}:                              totalTrainCost,
	})
	firstResetUpdated := requireStaffCommonResponse(t, "Staff.Reset", firstReset.GetCommonResponse())
	assertAllStaffReset(t, firstResetUpdated.GetStaffs(), catalog.trainings)
	if firstResetUpdated.GetStaffTrainingReset() == nil ||
		firstResetUpdated.GetStaffTrainingReset().GetStaffTrainingResetCount() != 0 {
		t.Fatalf("Staff.Reset reset state = %v, want unlimited count 0", firstResetUpdated.GetStaffTrainingReset())
	}
	if firstResetUpdated.GetTotalCount().GetStaffTrainCount() != wantHighWater {
		t.Fatalf("Staff.Reset changed historical train count to %d, want %d",
			firstResetUpdated.GetTotalCount().GetStaffTrainCount(), wantHighWater)
	}
	if firstResetUpdated.GetTotalCount().GetPointGoldAmount() != earnedGoldBefore {
		t.Fatalf("Staff.Reset counted refunded Gold as earnings: got=%d want=%d",
			firstResetUpdated.GetTotalCount().GetPointGoldAmount(), earnedGoldBefore)
	}
	assertNoStaffCardMutation(t, "Staff.Reset", firstResetUpdated)
	assertStoredStaffResources(t, dbCtx, seedGuildMedals, seedGold, seedLimitBreakItems)
	assertStoredStaffTrainCount(t, dbCtx, wantHighWater)
	assertStoredStaffResetCount(t, dbCtx, 0)
	assertStoredStaffsReset(t, dbCtx, catalog.trainings)
	assertStaffBalanceUnchanged(t, dbCtx, freeBalanceBefore, paidBalanceBefore)

	secondTrain := staffCall(t, func(ctx context.Context) (*papi.StaffTrainResponse, error) {
		return client.Train(ctx, &papi.StaffTrainRequest{
			ParameterType: catalog.parameterType,
			TrainCount:    1,
		})
	})
	secondTrainUpdated := requireStaffCommonResponse(t, "second Staff.Train", secondTrain.GetCommonResponse())
	assertStaff(t, findStaff(secondTrainUpdated.GetStaffs(), catalog.parameterType), 2, 0)
	assertNoStaffCardMutation(t, "second Staff.Train", secondTrainUpdated)
	assertStoredStaffResources(t, dbCtx, seedGuildMedals-secondTrainCost, seedGold, seedLimitBreakItems)
	assertStoredStaffTrainCount(t, dbCtx, wantHighWater)
	if updatedTotal := secondTrainUpdated.GetTotalCount(); updatedTotal != nil &&
		updatedTotal.GetStaffTrainCount() != wantHighWater {
		t.Fatalf("second Staff.Train returned historical train count %d, want %d",
			updatedTotal.GetStaffTrainCount(), wantHighWater)
	}

	secondReset := staffCall(t, func(ctx context.Context) (*papi.StaffResetResponse, error) {
		return client.Reset(ctx, &papi.Empty{})
	})
	assertStaffResetRewards(t, secondReset.GetRewardResults(), map[staffRewardKey]int64{
		{resourceType: penum.ResourceType_ResourceType_GuildMedal}: secondTrainCost,
	})
	secondResetUpdated := requireStaffCommonResponse(t, "second Staff.Reset", secondReset.GetCommonResponse())
	assertAllStaffReset(t, secondResetUpdated.GetStaffs(), catalog.trainings)
	if secondResetUpdated.GetStaffTrainingReset() == nil ||
		secondResetUpdated.GetStaffTrainingReset().GetStaffTrainingResetCount() != 0 {
		t.Fatalf("second Staff.Reset reset state = %v, want unlimited count 0", secondResetUpdated.GetStaffTrainingReset())
	}
	if secondResetUpdated.GetTotalCount().GetStaffTrainCount() != wantHighWater {
		t.Fatalf("second Staff.Reset changed historical train count to %d, want %d",
			secondResetUpdated.GetTotalCount().GetStaffTrainCount(), wantHighWater)
	}
	if secondResetUpdated.GetTotalCount().GetPointGoldAmount() != earnedGoldBefore {
		t.Fatalf("second Staff.Reset counted refunded Gold as earnings: got=%d want=%d",
			secondResetUpdated.GetTotalCount().GetPointGoldAmount(), earnedGoldBefore)
	}
	assertNoStaffCardMutation(t, "second Staff.Reset", secondResetUpdated)
	assertStoredStaffResources(t, dbCtx, seedGuildMedals, seedGold, seedLimitBreakItems)
	assertStoredStaffTrainCount(t, dbCtx, wantHighWater)
	assertStoredStaffResetCount(t, dbCtx, 0)
	assertStoredStaffsReset(t, dbCtx, catalog.trainings)
	assertStaffBalanceUnchanged(t, dbCtx, freeBalanceBefore, paidBalanceBefore)

	if err := opuser.UpsertUserStaffTrainingReset(dbCtx, testConfig.TestUserSuid, &ptransaction.UserStaffTrainingReset{
		StaffTrainingResetCount: 2,
	}); err != nil {
		t.Fatal(err)
	}
	finalUserGet := staffCall(t, func(ctx context.Context) (*papi.UserGetResponse, error) {
		return papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	})
	assertAllStaffReset(t, finalUserGet.GetStaffs(), catalog.trainings)
	if finalUserGet.GetStaffTrainingReset() == nil ||
		finalUserGet.GetStaffTrainingReset().GetStaffTrainingResetCount() != 0 {
		t.Fatalf("User.Get did not normalize imported reset state: %v", finalUserGet.GetStaffTrainingReset())
	}
	assertStoredStaffResetCount(t, dbCtx, 0)
	if finalUserGet.GetTotalCount().GetStaffTrainCount() != wantHighWater {
		t.Fatalf("User.Get historical train count = %d, want %d",
			finalUserGet.GetTotalCount().GetStaffTrainCount(), wantHighWater)
	}
	if got := findPoint(finalUserGet.GetPoints(), penum.PointType_PointType_GuildMedal).GetAmount(); got != seedGuildMedals {
		t.Fatalf("User.Get guild medals after second reset = %d, want %d", got, seedGuildMedals)
	}
	if got := findPoint(finalUserGet.GetPoints(), penum.PointType_PointType_Gold).GetAmount(); got != seedGold {
		t.Fatalf("User.Get gold after second reset = %d, want %d", got, seedGold)
	}
	if got := findItemAmount(finalUserGet.GetItems(), staffLimitBreakItemID); got != seedLimitBreakItems {
		t.Fatalf("User.Get limit-break items after second reset = %d, want %d", got, seedLimitBreakItems)
	}
	if finalUserGet.GetBalance().GetFreeBalance() != freeBalanceBefore ||
		finalUserGet.GetBalance().GetPaidBalance() != paidBalanceBefore {
		t.Fatalf("User.Get balance after free resets = %v, want free=%d paid=%d",
			finalUserGet.GetBalance(), freeBalanceBefore, paidBalanceBefore)
	}
}

func loadStaffScenarioCatalog(t *testing.T, ctx context.Context) staffScenarioCatalog {
	t.Helper()
	trainings, err := opmaster.FindAllStaffTrainings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, training := range trainings {
		parameterType := training.GetParameterType()
		rankZero, err := opmaster.FindStaffLevelLimitBreakRank(ctx, parameterType, 0)
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		rankOne, err := opmaster.FindStaffLevelLimitBreakRank(ctx, parameterType, 1)
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if rankZero.GetLevelLimit() <= 3 || rankOne.GetLevelLimit() <= rankZero.GetLevelLimit() ||
			rankOne.GetConsumptionItemAmount() <= 0 || rankOne.GetConsumptionGoldAmount() <= 0 {
			continue
		}
		levels, err := opmaster.FindStaffLevels(ctx, parameterType)
		if err != nil {
			t.Fatal(err)
		}
		byLevel := make(map[int32]*pmaster.StaffLevel, len(levels))
		for _, level := range levels {
			byLevel[level.GetLevel()] = level
		}
		complete := true
		for level := int32(1); level <= rankOne.GetLevelLimit(); level++ {
			master := byLevel[level]
			if master == nil || master.GetRequiredMedalAmount() < 0 {
				complete = false
				break
			}
		}
		if complete && staffMedalCost(t, byLevel, 1, 3) > 0 {
			return staffScenarioCatalog{
				parameterType: parameterType,
				trainings:     trainings,
				levels:        byLevel,
				rankZero:      rankZero,
				rankOne:       rankOne,
			}
		}
	}
	t.Fatal("master database has no complete Staff training catalog with ranks 0 and 1")
	return staffScenarioCatalog{}
}

func staffMedalCost(t *testing.T, levels map[int32]*pmaster.StaffLevel, fromLevel, toLevel int32) int64 {
	t.Helper()
	var cost int64
	for level := fromLevel + 1; level <= toLevel; level++ {
		master := levels[level]
		if master == nil {
			t.Fatalf("StaffLevel master is missing level %d", level)
		}
		cost += int64(master.GetRequiredMedalAmount())
	}
	return cost
}

func satisfyStaffLevelConditionForTest(t *testing.T, ctx context.Context, level *pmaster.StaffLevel) {
	t.Helper()
	if level == nil || level.GetUnlockConditionId() == "" {
		return
	}
	condition, err := opmaster.FindCondition(ctx, level.GetUnlockConditionId())
	if err != nil {
		t.Fatal(err)
	}
	areas := make([]*ptransaction.UserArea, 0, len(condition.GetSettings()))
	for _, setting := range condition.GetSettings() {
		clearArea := setting.GetClearArea()
		if clearArea == nil || clearArea.GetAreaId() == "" {
			continue
		}
		area, err := opmaster.FindArea(ctx, clearArea.GetAreaId())
		if err != nil {
			t.Fatal(err)
		}
		areas = append(areas, &ptransaction.UserArea{
			AreaId:          area.GetId(),
			ClearQuestCount: int32(len(area.GetQuestIds())),
			LastClearedTime: time.Now().UnixMilli(),
		})
	}
	if len(areas) > 0 {
		if err := opuser.UnlockUserAreasForGod(ctx, testConfig.TestUserSuid, areas); err != nil {
			t.Fatal(err)
		}
	}
}

func staffCall[T any](t *testing.T, call func(context.Context) (T, error)) T {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	response, err := call(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func requireStaffCommonResponse(t *testing.T, api string, response *papi.CommonResponse) *papi.UpdatedData {
	t.Helper()
	if response == nil || response.GetUpdatedData() == nil || response.GetDeletedData() == nil {
		t.Fatalf("%s returned an incomplete common response: %v", api, response)
	}
	return response.GetUpdatedData()
}

func findStaff(staffs []*ptransaction.UserStaff, parameterType penum.ParameterType) *ptransaction.UserStaff {
	for _, staff := range staffs {
		if staff.GetParameterType() == parameterType {
			return staff
		}
	}
	return nil
}

func assertStaff(t *testing.T, staff *ptransaction.UserStaff, level, rank int32) {
	t.Helper()
	if staff == nil || staff.GetLevel() != level || staff.GetLevelLimitBreakRank() != rank {
		t.Fatalf("Staff state = %v, want level=%d rank=%d", staff, level, rank)
	}
}

func assertAllStaffReset(
	t *testing.T,
	staffs []*ptransaction.UserStaff,
	trainings []*pmaster.StaffTraining,
) {
	t.Helper()
	if len(staffs) != len(trainings) {
		t.Fatalf("reset Staff count = %d, want current master count %d", len(staffs), len(trainings))
	}
	for _, training := range trainings {
		assertStaff(t, findStaff(staffs, training.GetParameterType()), 1, 0)
	}
}

func assertStoredStaffState(
	t *testing.T,
	ctx context.Context,
	parameterType penum.ParameterType,
	level, rank int32,
) {
	t.Helper()
	staff, err := opuser.FindUserStaff(ctx, testConfig.TestUserSuid, parameterType)
	if err != nil {
		t.Fatal(err)
	}
	assertStaff(t, staff, level, rank)
}

func assertStoredStaffsReset(t *testing.T, ctx context.Context, trainings []*pmaster.StaffTraining) {
	t.Helper()
	staffs, err := opuser.FindAllUserStaffs(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertAllStaffReset(t, staffs, trainings)
}

func findPoint(points []*ptransaction.UserPoint, pointType penum.PointType) *ptransaction.UserPoint {
	for _, point := range points {
		if point.GetPointType() == pointType {
			return point
		}
	}
	return nil
}

func findItemAmount(items []*ptransaction.UserItem, itemID string) int64 {
	var amount int64
	for _, item := range items {
		if item.GetItemId() == itemID {
			amount += item.GetAmount()
		}
	}
	return amount
}

func staffPointAmount(t *testing.T, ctx context.Context, pointType penum.PointType) int64 {
	t.Helper()
	point, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, pointType)
	if err != nil {
		t.Fatal(err)
	}
	return point.GetAmount()
}

func staffItemAmount(t *testing.T, ctx context.Context, itemID string) int64 {
	t.Helper()
	items, err := opuser.FindUserItems(ctx, testConfig.TestUserSuid, itemID)
	if err != nil {
		t.Fatal(err)
	}
	return findItemAmount(items, itemID)
}

func assertStoredStaffResources(t *testing.T, ctx context.Context, guildMedals, gold, items int64) {
	t.Helper()
	if got := staffPointAmount(t, ctx, penum.PointType_PointType_GuildMedal); got != guildMedals {
		t.Fatalf("persisted guild medals = %d, want %d", got, guildMedals)
	}
	if got := staffPointAmount(t, ctx, penum.PointType_PointType_Gold); got != gold {
		t.Fatalf("persisted gold = %d, want %d", got, gold)
	}
	if got := staffItemAmount(t, ctx, staffLimitBreakItemID); got != items {
		t.Fatalf("persisted limit-break items = %d, want %d", got, items)
	}
}

func assertStoredStaffTrainCount(t *testing.T, ctx context.Context, want int64) {
	t.Helper()
	total, err := opuser.FindUserTotalCount(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if total.GetStaffTrainCount() != want {
		t.Fatalf("persisted historical Staff train count = %d, want %d", total.GetStaffTrainCount(), want)
	}
}

func assertStoredStaffResetCount(t *testing.T, ctx context.Context, want int32) {
	t.Helper()
	reset, err := opuser.FindUserStaffTrainingReset(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if reset.GetStaffTrainingResetCount() != want {
		t.Fatalf("persisted Staff reset count = %d, want %d", reset.GetStaffTrainingResetCount(), want)
	}
}

func assertStaffBalanceUnchanged(t *testing.T, ctx context.Context, free, paid int32) {
	t.Helper()
	balance, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if balance.GetFreeBalance() != free || balance.GetPaidBalance() != paid {
		t.Fatalf("Staff.Reset changed dia balance to %v, want free=%d paid=%d", balance, free, paid)
	}
}

func assertNoStaffCardMutation(t *testing.T, api string, updated *papi.UpdatedData) {
	t.Helper()
	if len(updated.GetCards()) != 0 {
		t.Fatalf("%s rewrote card rows for a global Staff advantage: %v", api, updated.GetCards())
	}
}

type staffRewardKey struct {
	resourceType penum.ResourceType
	resourceID   string
}

func assertStaffResetRewards(
	t *testing.T,
	results []*ptransaction.RewardResult,
	want map[staffRewardKey]int64,
) {
	t.Helper()
	for key, amount := range want {
		if amount == 0 {
			delete(want, key)
		}
	}
	got := make(map[staffRewardKey]int64, len(results))
	for _, result := range results {
		key := staffRewardKey{resourceType: result.GetResourceType(), resourceID: result.GetResourceId()}
		delta := result.GetAfterAmount() - result.GetBeforeAmount()
		if result.GetAmountI64() != delta {
			t.Fatalf("Staff.Reset reward result amount mismatch: %v", result)
		}
		got[key] += delta
	}
	if len(got) != len(want) {
		t.Fatalf("Staff.Reset reward kinds = %v, want %v", got, want)
	}
	for key, amount := range want {
		if got[key] != amount {
			t.Fatalf("Staff.Reset reward %v = %d, want %d", key, got[key], amount)
		}
	}
}
