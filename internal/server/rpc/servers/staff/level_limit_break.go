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

func (s *StaffServer) LevelLimitBreak(
	ctx context.Context,
	req *papi.StaffLevelLimitBreakRequest,
) (*papi.StaffLevelLimitBreakResponse, error) {
	if req == nil || req.GetParameterType() == penum.ParameterType_ParameterType_Unknown {
		return nil, viber.InvalidArgumentf("parameterType is required")
	}
	if err := validateStaffParameter(ctx, req.GetParameterType()); err != nil {
		return nil, err
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.StaffLevelLimitBreakResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		staff, err := opuser.FindUserStaff(ctx, suid, req.GetParameterType())
		if err != nil {
			return nil, err
		}
		if staff.GetLevel() < 1 || staff.GetLevelLimitBreakRank() < 0 {
			return nil, viber.FailedPreconditionf("user staff state is invalid")
		}
		currentRank, err := opmaster.FindStaffLevelLimitBreakRank(
			ctx, staff.GetParameterType(), staff.GetLevelLimitBreakRank(),
		)
		if err != nil {
			return nil, err
		}
		if staff.GetLevel() != currentRank.GetLevelLimit() {
			return nil, viber.FailedPreconditionf("staff must reach its current level limit first")
		}
		if staff.GetLevelLimitBreakRank() == math.MaxInt32 {
			return nil, viber.FailedPreconditionf("staff potential rank limit reached")
		}
		nextRank, err := opmaster.FindStaffLevelLimitBreakRank(
			ctx, staff.GetParameterType(), staff.GetLevelLimitBreakRank()+1,
		)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, viber.FailedPreconditionf("staff potential rank limit reached")
			}
			return nil, err
		}
		if nextRank.GetRank() != staff.GetLevelLimitBreakRank()+1 ||
			nextRank.GetLevelLimit() <= currentRank.GetLevelLimit() ||
			nextRank.GetConsumptionItemAmount() < 0 || nextRank.GetConsumptionGoldAmount() < 0 {
			return nil, viber.FailedPreconditionf("staff potential rank master data is invalid")
		}

		levels, err := opmaster.FindStaffLevels(ctx, staff.GetParameterType())
		if err != nil {
			return nil, err
		}
		levelByNumber := make(map[int32]*pmaster.StaffLevel, len(levels))
		for _, level := range levels {
			levelByNumber[level.GetLevel()] = level
		}
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
		for number := currentRank.GetLevelLimit() + 1; number <= nextRank.GetLevelLimit(); number++ {
			level := levelByNumber[number]
			if level == nil {
				return nil, viber.FailedPreconditionf("new staff level limit is missing from master data")
			}
			unlocked, err := evaluator.Satisfied(level.GetUnlockConditionId())
			if err != nil {
				return nil, err
			}
			if !unlocked {
				return nil, viber.FailedPreconditionf("staff level %d is locked", number)
			}
		}

		updated := &papi.UpdatedData{}
		if nextRank.GetConsumptionItemAmount() > 0 {
			if err := viber.ConsumeExchangeResource(
				ctx,
				suid,
				penum.ResourceType_ResourceType_Item,
				staffLevelLimitBreakItemID,
				nextRank.GetConsumptionItemAmount(),
				updated,
			); err != nil {
				return nil, err
			}
		}
		if nextRank.GetConsumptionGoldAmount() > 0 {
			if err := viber.ConsumeExchangeResource(
				ctx,
				suid,
				penum.ResourceType_ResourceType_Gold,
				"",
				nextRank.GetConsumptionGoldAmount(),
				updated,
			); err != nil {
				return nil, err
			}
		}

		staff.LevelLimitBreakRank = nextRank.GetRank()
		if err := opuser.UpsertUserStaff(ctx, suid, staff); err != nil {
			return nil, err
		}
		updated.Staffs = []*ptransaction.UserStaff{staff}
		response = &papi.StaffLevelLimitBreakResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, staffMutationError(err, "insufficient staff potential materials")
	}
	return response, nil
}
