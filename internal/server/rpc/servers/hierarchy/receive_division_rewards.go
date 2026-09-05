package hierarchy

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *HierarchyServer) ReceiveDivisionRewards(
	ctx context.Context,
	req *papi.HierarchyDivisionRewardsReceiveRequest,
) (*papi.HierarchyDivisionRewardsReceiveResponse, error) {
	if err := validateHierarchyRewardIDs(req.GetHierarchyDivisionRewardIds(), "hierarchyDivisionRewardIds"); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.HierarchyDivisionRewardsReceiveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		overview, err := loadHierarchyOverview(ctx, suid)
		if err != nil {
			return nil, err
		}
		rewardsByID := make(map[string]*pmaster.HierarchyDivisionReward, len(overview.divisionRewards))
		for _, reward := range overview.divisionRewards {
			rewardsByID[reward.GetId()] = reward
		}
		received := hierarchyStringSet(overview.hierarchy.GetReceivedDivisionRewardIds())
		selected := make([]*pmaster.HierarchyDivisionReward, 0, len(req.GetHierarchyDivisionRewardIds()))
		for _, rewardID := range req.GetHierarchyDivisionRewardIds() {
			reward := rewardsByID[rewardID]
			if reward == nil {
				return nil, viber.InvalidArgumentf("unknown Hierarchy division reward %q", rewardID)
			}
			if _, exists := received[rewardID]; exists {
				return nil, viber.FailedPreconditionf("Hierarchy division reward %q was already received", rewardID)
			}
			if !overview.divisionRewardReached[rewardID] {
				return nil, viber.FailedPreconditionf("Hierarchy division reward %q is not reached", rewardID)
			}
			selected = append(selected, reward)
			received[rewardID] = struct{}{}
			overview.hierarchy.ReceivedDivisionRewardIds = append(overview.hierarchy.ReceivedDivisionRewardIds, rewardID)
		}
		sort.Strings(overview.hierarchy.ReceivedDivisionRewardIds)
		if err := opuser.UpsertUserHierarchy(ctx, suid, overview.hierarchy); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{Hierarchy: overview.hierarchy}
		results := make([]*ptransaction.RewardResult, 0)
		for _, reward := range selected {
			granted, err := grantHierarchyReward(ctx, suid, reward.GetRewardId(), updated)
			if err != nil {
				return nil, err
			}
			results = append(results, granted...)
		}
		postOverview, err := loadHierarchyOverview(ctx, suid)
		if err != nil {
			return nil, err
		}
		if postOverview.hierarchyChanged {
			if err := opuser.UpsertUserHierarchy(ctx, suid, postOverview.hierarchy); err != nil {
				return nil, err
			}
		}
		updated.Hierarchy = postOverview.hierarchy
		if err := persistHierarchyNotification(ctx, suid, postOverview); err != nil {
			return nil, err
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.HierarchyDivisionRewardsReceiveResponse{
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
