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

func (s *HierarchyServer) ReceiveRewards(
	ctx context.Context,
	req *papi.HierarchyRewardsReceiveRequest,
) (*papi.HierarchyRewardsReceiveResponse, error) {
	if err := validateHierarchyRewardIDs(req.GetHierarchyRewardIds(), "hierarchyRewardIds"); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.HierarchyRewardsReceiveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		overview, err := loadHierarchyOverview(ctx, suid)
		if err != nil {
			return nil, err
		}
		rewardsByID := make(map[string]*pmaster.HierarchyReward, len(overview.rewards))
		for _, reward := range overview.rewards {
			rewardsByID[reward.GetId()] = reward
		}
		received := hierarchyStringSet(overview.hierarchy.GetReceivedRewardIds())
		selected := make([]*pmaster.HierarchyReward, 0, len(req.GetHierarchyRewardIds()))
		for _, rewardID := range req.GetHierarchyRewardIds() {
			reward := rewardsByID[rewardID]
			if reward == nil {
				return nil, viber.InvalidArgumentf("unknown Hierarchy reward %q", rewardID)
			}
			if _, exists := received[rewardID]; exists {
				return nil, viber.FailedPreconditionf("Hierarchy reward %q was already received", rewardID)
			}
			if !overview.rewardReached[rewardID] {
				return nil, viber.FailedPreconditionf("Hierarchy reward %q is not reached", rewardID)
			}
			selected = append(selected, reward)
			received[rewardID] = struct{}{}
			overview.hierarchy.ReceivedRewardIds = append(overview.hierarchy.ReceivedRewardIds, rewardID)
		}
		sort.Strings(overview.hierarchy.ReceivedRewardIds)
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
		response = &papi.HierarchyRewardsReceiveResponse{
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
