package staff

import (
	"context"
	"errors"
	"math"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *StaffServer) Reset(ctx context.Context, _ *papi.Empty) (*papi.StaffResetResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.StaffResetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		trainings, err := opmaster.FindAllStaffTrainings(ctx)
		if err != nil {
			return nil, err
		}
		levels, err := opmaster.FindAllStaffLevels(ctx)
		if err != nil {
			return nil, err
		}
		ranks, err := opmaster.FindAllStaffLevelLimitBreakRanks(ctx)
		if err != nil {
			return nil, err
		}
		staffs, err := opuser.FindAllUserStaffs(ctx, suid)
		if err != nil {
			return nil, err
		}
		resetStaffs, costs, err := buildStaffResetPlan(trainings, levels, ranks, staffs)
		if err != nil {
			return nil, err
		}

		updated := &papi.UpdatedData{}
		rewardResults, err := refundStaffResetCosts(ctx, suid, costs, updated)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertManyUserStaffs(ctx, suid, resetStaffs); err != nil {
			return nil, err
		}
		updated.Staffs = resetStaffs

		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			totalCount = &ptransaction.UserTotalCount{}
			if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
				return nil, err
			}
		}
		updated.TotalCount = totalCount

		// Keeping this at zero makes every reset look like the first, free reset
		// to the official client and removes both the monthly charge and limit.
		resetState := &ptransaction.UserStaffTrainingReset{StaffTrainingResetCount: 0}
		if err := opuser.UpsertUserStaffTrainingReset(ctx, suid, resetState); err != nil {
			return nil, err
		}
		updated.StaffTrainingReset = resetState

		response = &papi.StaffResetResponse{
			RewardResults:  rewardResults,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

type staffResetCosts struct {
	limitBreakItems int64
	gold            int64
	guildMedals     int64
}

func buildStaffResetPlan(
	trainings []*pmaster.StaffTraining,
	levels []*pmaster.StaffLevel,
	ranks []*pmaster.StaffLevelLimitBreakRank,
	staffs []*ptransaction.UserStaff,
) ([]*ptransaction.UserStaff, staffResetCosts, error) {
	levelByParameter := make(map[penum.ParameterType]map[int32]*pmaster.StaffLevel)
	for _, level := range levels {
		if level == nil || level.GetLevel() < 1 {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("staff level master data is invalid")
		}
		values := levelByParameter[level.GetParameterType()]
		if values == nil {
			values = make(map[int32]*pmaster.StaffLevel)
			levelByParameter[level.GetParameterType()] = values
		}
		if _, exists := values[level.GetLevel()]; exists {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("staff level master data is duplicated")
		}
		values[level.GetLevel()] = level
	}

	rankByParameter := make(map[penum.ParameterType]map[int32]*pmaster.StaffLevelLimitBreakRank)
	for _, rank := range ranks {
		if rank == nil || rank.GetRank() < 0 {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("staff potential rank master data is invalid")
		}
		values := rankByParameter[rank.GetParameterType()]
		if values == nil {
			values = make(map[int32]*pmaster.StaffLevelLimitBreakRank)
			rankByParameter[rank.GetParameterType()] = values
		}
		if _, exists := values[rank.GetRank()]; exists {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("staff potential rank master data is duplicated")
		}
		values[rank.GetRank()] = rank
	}

	staffByParameter := make(map[penum.ParameterType]*ptransaction.UserStaff, len(staffs))
	for _, staff := range staffs {
		if staff == nil {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("user staff state is invalid")
		}
		if _, exists := staffByParameter[staff.GetParameterType()]; exists {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("user staff state is duplicated")
		}
		staffByParameter[staff.GetParameterType()] = staff
	}

	resetStaffs := make([]*ptransaction.UserStaff, 0, len(trainings))
	knownParameters := make(map[penum.ParameterType]struct{}, len(trainings))
	var costs staffResetCosts
	for _, training := range trainings {
		if training == nil || training.GetParameterType() == penum.ParameterType_ParameterType_Unknown {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("staff training master data is invalid")
		}
		parameterType := training.GetParameterType()
		if _, exists := knownParameters[parameterType]; exists {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("staff training master data is duplicated")
		}
		knownParameters[parameterType] = struct{}{}

		staff := staffByParameter[parameterType]
		parameterLevels := levelByParameter[parameterType]
		parameterRanks := rankByParameter[parameterType]
		if staff == nil || len(parameterLevels) == 0 || len(parameterRanks) == 0 {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("staff state or master data is incomplete")
		}
		baselineLevel, baselineRank := minimumStaffLevel(parameterLevels), minimumStaffRank(parameterRanks)
		if staff.GetLevel() < baselineLevel || staff.GetLevelLimitBreakRank() < baselineRank {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("user staff state is below its baseline")
		}
		currentRank := parameterRanks[staff.GetLevelLimitBreakRank()]
		if currentRank == nil || staff.GetLevel() > currentRank.GetLevelLimit() {
			return nil, staffResetCosts{}, viber.FailedPreconditionf("user staff state exceeds its level limit")
		}

		for number := baselineLevel + 1; number <= staff.GetLevel(); number++ {
			level := parameterLevels[number]
			if level == nil || level.GetRequiredMedalAmount() < 0 {
				return nil, staffResetCosts{}, viber.FailedPreconditionf("trained staff levels are not contiguous")
			}
			if err := addStaffResetCost(&costs.guildMedals, int64(level.GetRequiredMedalAmount())); err != nil {
				return nil, staffResetCosts{}, err
			}
		}
		for rank := baselineRank + 1; rank <= staff.GetLevelLimitBreakRank(); rank++ {
			value := parameterRanks[rank]
			if value == nil || value.GetConsumptionItemAmount() < 0 || value.GetConsumptionGoldAmount() < 0 {
				return nil, staffResetCosts{}, viber.FailedPreconditionf("staff potential ranks are not contiguous")
			}
			if err := addStaffResetCost(&costs.limitBreakItems, value.GetConsumptionItemAmount()); err != nil {
				return nil, staffResetCosts{}, err
			}
			if err := addStaffResetCost(&costs.gold, value.GetConsumptionGoldAmount()); err != nil {
				return nil, staffResetCosts{}, err
			}
		}
		resetStaffs = append(resetStaffs, &ptransaction.UserStaff{
			ParameterType:       parameterType,
			Level:               baselineLevel,
			LevelLimitBreakRank: baselineRank,
		})
	}
	if len(staffByParameter) != len(knownParameters) {
		return nil, staffResetCosts{}, viber.FailedPreconditionf("user staff state contains unknown parameters")
	}
	return resetStaffs, costs, nil
}

func minimumStaffLevel(values map[int32]*pmaster.StaffLevel) int32 {
	minimum := int32(math.MaxInt32)
	for level := range values {
		if level < minimum {
			minimum = level
		}
	}
	return minimum
}

func minimumStaffRank(values map[int32]*pmaster.StaffLevelLimitBreakRank) int32 {
	minimum := int32(math.MaxInt32)
	for rank := range values {
		if rank < minimum {
			minimum = rank
		}
	}
	return minimum
}

func addStaffResetCost(total *int64, amount int64) error {
	if amount < 0 || *total > math.MaxInt64-amount {
		return viber.FailedPreconditionf("staff reset refund overflows")
	}
	*total += amount
	return nil
}

func refundStaffResetCosts(
	ctx context.Context,
	suid string,
	costs staffResetCosts,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if costs.limitBreakItems > math.MaxInt32 || costs.guildMedals > math.MaxInt32 {
		return nil, viber.FailedPreconditionf("staff reset refund is too large")
	}
	results := make([]*ptransaction.RewardResult, 0, 3)
	if costs.limitBreakItems > 0 {
		granted, err := viber.GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   staffLevelLimitBreakItemID,
			Amount:       int32(costs.limitBreakItems),
		}, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, granted...)
	}
	if costs.gold > 0 {
		result, err := refundStaffResetPoint(
			ctx, suid, penum.PointType_PointType_Gold, penum.ResourceType_ResourceType_Gold, costs.gold, updated,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if costs.guildMedals > 0 {
		granted, err := viber.GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_GuildMedal,
			Amount:       int32(costs.guildMedals),
		}, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, granted...)
	}
	return results, nil
}

func refundStaffResetPoint(
	ctx context.Context,
	suid string,
	pointType penum.PointType,
	resourceType penum.ResourceType,
	amount int64,
	updated *papi.UpdatedData,
) (*ptransaction.RewardResult, error) {
	point, err := opuser.FindUserPoint(ctx, suid, pointType)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		point = &ptransaction.UserPoint{PointType: pointType}
	}
	before := point.GetAmount()
	if before < 0 || amount <= 0 || before > math.MaxInt64-amount {
		return nil, viber.FailedPreconditionf("staff reset point refund is invalid")
	}
	point.Amount += amount
	if err := opuser.UpsertUserPoint(ctx, suid, point); err != nil {
		return nil, err
	}
	updated.Points = append(updated.Points, point)
	return &ptransaction.RewardResult{
		ResourceType: resourceType,
		BeforeAmount: before,
		AfterAmount:  point.GetAmount(),
		AmountI64:    amount,
		IsNew:        before == 0,
	}, nil
}
