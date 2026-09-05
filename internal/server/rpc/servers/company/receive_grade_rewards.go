package company

import (
	"context"
	"errors"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *CompanyServer) ReceiveGradeRewards(
	ctx context.Context,
	req *papi.CompanyReceiveGradeRewardsRequest,
) (*papi.CompanyReceiveGradeRewardsResponse, error) {
	if err := validateUniqueCompanyGradeIDs(req.GetGradeIds()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyReceiveGradeRewardsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		company, err := opuser.FindUserCompany(ctx, suid)
		if err != nil {
			return nil, err
		}
		totalPoint, err := companyTotalPoint(company)
		if err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		received := stringSet(state.ReceivedGradeIds)
		updated := &papi.UpdatedData{Company: company}
		results := make([]*ptransaction.RewardResult, 0)
		for _, gradeID := range req.GetGradeIds() {
			grade, err := opmaster.FindCompanyGrade(ctx, gradeID)
			if err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, viber.InvalidArgumentf("unknown Company grade %q", gradeID)
				}
				return nil, err
			}
			if grade.GetAchieveRewardId() == "" {
				return nil, viber.FailedPreconditionf("Company grade %q has no reward", gradeID)
			}
			if _, exists := received[gradeID]; exists {
				return nil, viber.FailedPreconditionf("Company grade %q reward was already received", gradeID)
			}
			if totalPoint < grade.GetRequiredPoint() {
				return nil, viber.FailedPreconditionf("Company grade %q is not reached", gradeID)
			}
			granted, err := grantCompanyRewardSet(ctx, suid, grade.GetAchieveRewardId(), updated)
			if err != nil {
				return nil, err
			}
			results = append(results, granted...)
			received[gradeID] = struct{}{}
			state.ReceivedGradeIds = append(state.ReceivedGradeIds, gradeID)
		}
		sort.Strings(state.ReceivedGradeIds)
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyReceiveGradeRewardsResponse{
			Results:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func validateUniqueCompanyGradeIDs(gradeIDs []string) error {
	if len(gradeIDs) == 0 {
		return viber.InvalidArgumentf("gradeIds is required")
	}
	seen := make(map[string]struct{}, len(gradeIDs))
	for _, gradeID := range gradeIDs {
		if gradeID == "" {
			return viber.InvalidArgumentf("gradeId must not be empty")
		}
		if _, exists := seen[gradeID]; exists {
			return viber.InvalidArgumentf("gradeIds must not contain duplicates")
		}
		seen[gradeID] = struct{}{}
	}
	return nil
}
