package hierarchy

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *HierarchyServer) Top(ctx context.Context, _ *papi.Empty) (*papi.HierarchyTopResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.HierarchyTopResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		overview, err := loadHierarchyOverview(ctx, suid)
		if err != nil {
			return nil, err
		}
		profile, err := viber.LoadSelfProfileInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		receivedRewards := hierarchyStringSet(overview.hierarchy.GetReceivedRewardIds())
		receivedDivisionRewards := hierarchyStringSet(overview.hierarchy.GetReceivedDivisionRewardIds())

		rewardInfos := make([]*papi.HierarchyRewardInfo, 0, len(overview.rewards))
		for _, reward := range overview.rewards {
			_, received := receivedRewards[reward.GetId()]
			rewardInfos = append(rewardInfos, &papi.HierarchyRewardInfo{
				HierarchyRewardId: reward.GetId(),
				CanReceive:        overview.rewardReached[reward.GetId()],
				HaveReceived:      received,
			})
		}

		divisionRewardInfos := make([]*papi.HierarchyDivisionRewardInfo, 0, len(overview.divisionRewards))
		for _, reward := range overview.divisionRewards {
			reached := overview.divisionRewardReached[reward.GetId()]
			_, received := receivedDivisionRewards[reward.GetId()]
			var firstUserInfo *papi.ProfileInfo
			if reached {
				firstUserInfo = profile
			}
			divisionRewardInfos = append(divisionRewardInfos, &papi.HierarchyDivisionRewardInfo{
				HierarchyDivisionRewardId: reward.GetId(),
				CanReceive:                reached,
				HaveReceived:              received,
				FirstUserInfo:             firstUserInfo,
			})
		}

		var updated *papi.UpdatedData
		if overview.hierarchyChanged {
			if err := opuser.UpsertUserHierarchy(ctx, suid, overview.hierarchy); err != nil {
				return nil, err
			}
			updated = &papi.UpdatedData{Hierarchy: overview.hierarchy}
		}
		if err := persistHierarchyNotification(ctx, suid, overview); err != nil {
			return nil, err
		}
		response = &papi.HierarchyTopResponse{
			UserHierarchyInfo: &papi.UserHierarchyInfo{
				CurrentDetailGradeId: overview.currentGrade.GetId(),
				CurrentRank:          viber.HierarchyPrivateServerRank,
				CurrentPoint:         overview.currentPoint,
			},
			RewardInfos:           rewardInfos,
			DivisionRewardInfos:   divisionRewardInfos,
			HighestGradeUserNames: []string{},
			CommonResponse:        viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
