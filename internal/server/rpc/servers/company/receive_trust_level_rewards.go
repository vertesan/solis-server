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

func (s *CompanyServer) ReceiveTrustLevelRewards(
	ctx context.Context,
	req *papi.CompanyReceiveTrustLevelRewardsRequest,
) (*papi.CompanyReceiveTrustLevelRewardsResponse, error) {
	if len(req.GetLevelKeys()) == 0 {
		return nil, viber.InvalidArgumentf("levelKeys is required")
	}
	if err := validateUniqueLevelKeys(req.GetLevelKeys()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyReceiveTrustLevelRewardsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		company, err := opuser.FindUserCompany(ctx, suid)
		if err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		received := int32Set(state.ReceivedTrustLevelKeys)
		updated := &papi.UpdatedData{Company: company}
		results := make([]*ptransaction.RewardResult, 0)
		for _, levelKey := range req.GetLevelKeys() {
			level, err := opmaster.FindCompanyTrustLevel(ctx, levelKey)
			if err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, viber.InvalidArgumentf("unknown Company trust level %d", levelKey)
				}
				return nil, err
			}
			if level.GetAchieveRewardId() == "" {
				return nil, viber.FailedPreconditionf("Company trust level %d has no reward", levelKey)
			}
			if _, exists := received[levelKey]; exists {
				return nil, viber.FailedPreconditionf("Company trust level %d reward was already received", levelKey)
			}
			if company.GetTrustPoint() < level.GetRequiredPoint() {
				return nil, viber.FailedPreconditionf("Company trust level %d is not reached", levelKey)
			}
			granted, err := grantCompanyRewardSet(ctx, suid, level.GetAchieveRewardId(), updated)
			if err != nil {
				return nil, err
			}
			results = append(results, granted...)
			received[levelKey] = struct{}{}
			state.ReceivedTrustLevelKeys = append(state.ReceivedTrustLevelKeys, levelKey)
		}
		sort.SliceStable(state.ReceivedTrustLevelKeys, func(i, j int) bool {
			return state.ReceivedTrustLevelKeys[i] < state.ReceivedTrustLevelKeys[j]
		})
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyReceiveTrustLevelRewardsResponse{
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
