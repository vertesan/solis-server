package staff

import (
	"context"
	"errors"
	"math"
	"time"

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

func (s *StaffServer) Train(ctx context.Context, req *papi.StaffTrainRequest) (*papi.StaffTrainResponse, error) {
	if req == nil || req.GetParameterType() == penum.ParameterType_ParameterType_Unknown || req.GetTrainCount() <= 0 {
		return nil, viber.InvalidArgumentf("parameterType and a positive trainCount are required")
	}
	if req.GetTrainCount() > math.MaxInt32 {
		return nil, viber.InvalidArgumentf("trainCount is too large")
	}
	if err := validateStaffParameter(ctx, req.GetParameterType()); err != nil {
		return nil, err
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.StaffTrainResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		staff, err := opuser.FindUserStaff(ctx, suid, req.GetParameterType())
		if err != nil {
			return nil, err
		}
		if staff.GetLevel() < 1 || staff.GetLevelLimitBreakRank() < 0 {
			return nil, viber.FailedPreconditionf("user staff state is invalid")
		}
		if req.GetTrainCount() > math.MaxInt32-int64(staff.GetLevel()) {
			return nil, viber.InvalidArgumentf("target staff level overflows")
		}

		rank, err := opmaster.FindStaffLevelLimitBreakRank(
			ctx, staff.GetParameterType(), staff.GetLevelLimitBreakRank(),
		)
		if err != nil {
			return nil, err
		}
		targetLevel := staff.GetLevel() + int32(req.GetTrainCount())
		if targetLevel > rank.GetLevelLimit() {
			return nil, viber.FailedPreconditionf(
				"staff level %d exceeds the current limit %d", targetLevel, rank.GetLevelLimit(),
			)
		}

		levels, err := opmaster.FindStaffLevels(ctx, staff.GetParameterType())
		if err != nil {
			return nil, err
		}
		levelByNumber := make(map[int32]*pmaster.StaffLevel, len(levels))
		for _, level := range levels {
			if level == nil || level.GetLevel() < 1 {
				return nil, viber.FailedPreconditionf("staff level master data is invalid")
			}
			if _, exists := levelByNumber[level.GetLevel()]; exists {
				return nil, viber.FailedPreconditionf("staff level master data is duplicated")
			}
			levelByNumber[level.GetLevel()] = level
		}
		if levelByNumber[staff.GetLevel()] == nil {
			return nil, viber.FailedPreconditionf("current staff level is missing from master data")
		}

		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
		var medalCost int64
		for number := staff.GetLevel() + 1; number <= targetLevel; number++ {
			level := levelByNumber[number]
			if level == nil {
				return nil, viber.FailedPreconditionf("target staff levels are not contiguous")
			}
			if level.GetRequiredMedalAmount() < 0 || medalCost > math.MaxInt64-int64(level.GetRequiredMedalAmount()) {
				return nil, viber.FailedPreconditionf("staff training medal cost is invalid")
			}
			unlocked, err := evaluator.Satisfied(level.GetUnlockConditionId())
			if err != nil {
				return nil, err
			}
			if !unlocked {
				return nil, viber.FailedPreconditionf("staff level %d is locked", number)
			}
			medalCost += int64(level.GetRequiredMedalAmount())
		}

		updated := &papi.UpdatedData{}
		if medalCost > 0 {
			if err := viber.ConsumeExchangeResource(
				ctx,
				suid,
				penum.ResourceType_ResourceType_GuildMedal,
				"",
				medalCost,
				updated,
			); err != nil {
				return nil, err
			}
		}

		staff.Level = targetLevel
		if err := opuser.UpsertUserStaff(ctx, suid, staff); err != nil {
			return nil, err
		}
		updated.Staffs = []*ptransaction.UserStaff{staff}

		allStaffs, err := opuser.FindAllUserStaffs(ctx, suid)
		if err != nil {
			return nil, err
		}
		currentEnhanceCount, err := viber.CurrentStaffEnhanceCount(allStaffs)
		if err != nil {
			return nil, err
		}
		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			totalCount = &ptransaction.UserTotalCount{}
		}
		if int64(currentEnhanceCount) > totalCount.GetStaffTrainCount() {
			totalCount.StaffTrainCount = int64(currentEnhanceCount)
			if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
				return nil, err
			}
			updated.TotalCount = totalCount
			missions, err := viber.ReachUserMissions(
				ctx,
				suid,
				penum.MissionType_MissionType_StaffTrainTotalCount,
				totalCount.GetStaffTrainCount(),
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = missions
		}

		response = &papi.StaffTrainResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, staffMutationError(err, "insufficient guild medals")
	}
	return response, nil
}
