package hierarchy

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

type hierarchyOverview struct {
	hierarchy             *ptransaction.UserHierarchy
	hierarchyChanged      bool
	currentGrade          *pmaster.HierarchyDetailGrade
	currentPoint          int64
	rewards               []*pmaster.HierarchyReward
	divisionRewards       []*pmaster.HierarchyDivisionReward
	rewardReached         map[string]bool
	divisionRewardReached map[string]bool
}

func loadHierarchyOverview(ctx context.Context, suid string) (*hierarchyOverview, error) {
	shared, err := viber.LoadHierarchyOverview(ctx, suid, viber.HierarchyPrivateServerRank)
	if err != nil {
		return nil, err
	}
	return &hierarchyOverview{
		hierarchy:             shared.Hierarchy,
		hierarchyChanged:      shared.HierarchyChanged,
		currentGrade:          shared.CurrentGrade,
		currentPoint:          shared.CurrentPoint,
		rewards:               shared.Rewards,
		divisionRewards:       shared.DivisionRewards,
		rewardReached:         shared.RewardReached,
		divisionRewardReached: shared.DivisionRewardReached,
	}, nil
}

func grantHierarchyReward(
	ctx context.Context,
	suid string,
	rewardID string,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	resources, err := viber.ResolveRewardResources(ctx, rewardID, 1)
	if err != nil {
		return nil, err
	}
	results := make([]*ptransaction.RewardResult, 0, len(resources))
	for _, resource := range resources {
		granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, granted...)
	}
	return results, nil
}

func persistHierarchyNotification(ctx context.Context, suid string, overview *hierarchyOverview) error {
	_, err := viber.PersistHierarchyNotification(ctx, suid, &viber.HierarchyOverview{
		Hierarchy:             overview.hierarchy,
		HierarchyChanged:      overview.hierarchyChanged,
		CurrentGrade:          overview.currentGrade,
		CurrentRank:           viber.HierarchyPrivateServerRank,
		CurrentPoint:          overview.currentPoint,
		Rewards:               overview.rewards,
		DivisionRewards:       overview.divisionRewards,
		RewardReached:         overview.rewardReached,
		DivisionRewardReached: overview.divisionRewardReached,
	})
	return err
}

func validateHierarchyRewardIDs(ids []string, field string) error {
	if len(ids) == 0 {
		return viber.InvalidArgumentf("%s is required", field)
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return viber.InvalidArgumentf("%s must not contain an empty ID", field)
		}
		if _, exists := seen[id]; exists {
			return viber.InvalidArgumentf("%s must not contain duplicates", field)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func hierarchyStringSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}
