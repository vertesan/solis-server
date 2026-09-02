package servers

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

const hierarchyPrivateServerRank int32 = 1

type HierarchyServer struct {
	papi.UnimplementedHierarchyServer
}

func (s *HierarchyServer) Top(ctx context.Context, _ *papi.Empty) (*papi.HierarchyTopResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.HierarchyTopResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		overview, err := loadHierarchyOverview(ctx, suid)
		if err != nil {
			return nil, err
		}
		profile, err := hierarchySelfProfile(ctx, suid)
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
				CurrentRank:          hierarchyPrivateServerRank,
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

func (s *HierarchyServer) ListRanking(ctx context.Context, _ *papi.Empty) (*papi.HierarchyRankingResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.HierarchyRankingResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		public, err := opuser.FindUserPublic(ctx, suid)
		if err != nil {
			return nil, err
		}
		hierarchy, err := opuser.FindUserHierarchy(ctx, suid)
		if err != nil {
			return nil, err
		}
		point, err := viber.HierarchyTotalPoint(hierarchy)
		if err != nil {
			return nil, err
		}
		grades, err := opmaster.FindAllHierarchyDetailGrades(ctx)
		if err != nil {
			return nil, err
		}
		grade, err := viber.ResolveHierarchyDetailGrade(grades, point, hierarchyPrivateServerRank)
		if err != nil {
			return nil, err
		}

		deckName, cardInfos, err := hierarchyRankingDeck(ctx, suid)
		if err != nil {
			return nil, err
		}
		response = &papi.HierarchyRankingResponse{
			RankUserInfos: []*papi.HierarchyRankUserInfo{{
				UserId:               public.GetPublicUserId(),
				CurrentRank:          hierarchyPrivateServerRank,
				ManagerName:          user.GetName(),
				ManagerLevel:         user.GetManagerLevel(),
				CurrentDetailGradeId: grade.GetId(),
				CurrentPoint:         point,
				EmblemId:             user.GetEmblemId(),
				DeckName:             deckName,
				CardInfos:            cardInfos,
			}},
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

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
	shared, err := viber.LoadHierarchyOverview(ctx, suid, hierarchyPrivateServerRank)
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

func hierarchyRankingDeck(ctx context.Context, suid string) (string, []*papi.LastCardInfo, error) {
	decks, err := opuser.FindAllUserDecks(ctx, suid)
	if err != nil {
		return "", nil, err
	}
	if len(decks) == 0 {
		return "", []*papi.LastCardInfo{}, nil
	}
	deck := decks[0]
	positions, err := opuser.FindUserDeckPositions(ctx, suid, deck.GetNumber())
	if err != nil {
		return "", nil, err
	}
	sort.SliceStable(positions, func(i, j int) bool { return positions[i].GetPosition() < positions[j].GetPosition() })
	cards, err := opuser.FindAllUserCards(ctx, suid)
	if err != nil {
		return "", nil, err
	}
	cardsByID := make(map[string]*ptransaction.UserCard, len(cards))
	for _, card := range cards {
		cardsByID[card.GetCardId()] = card
	}
	infos := make([]*papi.LastCardInfo, 0, len(positions))
	for _, position := range positions {
		card := cardsByID[position.GetCardId()]
		if card == nil {
			continue
		}
		infos = append(infos, &papi.LastCardInfo{
			Position:       position.GetPosition(),
			CardId:         card.GetCardId(),
			Level:          card.GetLevel(),
			Rarity:         card.GetRarity(),
			EvolutionLevel: card.GetEvolutionLevel(),
		})
	}
	return deck.GetName(), infos, nil
}

func hierarchySelfProfile(ctx context.Context, suid string) (*papi.ProfileInfo, error) {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, err
	}
	profile, err := opuser.FindUserProfile(ctx, suid)
	if err != nil {
		return nil, err
	}
	return &papi.ProfileInfo{
		UserId:               public.GetPublicUserId(),
		Name:                 user.GetName(),
		ManagerExp:           user.GetManagerExp(),
		ManagerLevel:         user.GetManagerLevel(),
		EmblemId:             user.GetEmblemId(),
		FavoriteCharacterIds: append([]string(nil), profile.GetFavoriteCharacterIds()...),
		BackgroundType:       profile.GetBackgroundType(),
		LayoutType:           profile.GetLayoutType(),
		LastLoginTime:        user.GetLastLoginTime(),
		DecorationId:         profile.GetDecorationId(),
		InformationType:      profile.GetInformationType(),
		ColorType:            profile.GetColorType(),
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
		CurrentRank:           hierarchyPrivateServerRank,
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
