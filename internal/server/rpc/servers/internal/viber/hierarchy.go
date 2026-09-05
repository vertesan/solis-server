package viber

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// HierarchyOverview is the master-derived hierarchy state for one user and
// rank. Reward eligibility is kept separate from receipt state so callers can
// expose both flags faithfully.
type HierarchyOverview struct {
	Hierarchy             *ptransaction.UserHierarchy
	HierarchyChanged      bool
	CurrentGrade          *pmaster.HierarchyDetailGrade
	CurrentRank           int32
	CurrentPoint          int64
	Rewards               []*pmaster.HierarchyReward
	DivisionRewards       []*pmaster.HierarchyDivisionReward
	RewardReached         map[string]bool
	DivisionRewardReached map[string]bool
}

// HierarchyTotalPoint returns the user's current Venus Point total.
func HierarchyTotalPoint(hierarchy *ptransaction.UserHierarchy) (int64, error) {
	if hierarchy == nil {
		return 0, fmt.Errorf("user hierarchy is missing")
	}
	fixed := hierarchy.GetCurrentFixedPoint()
	variable := hierarchy.GetCurrentVariablePoint()
	if fixed < 0 || variable < 0 {
		return 0, fmt.Errorf("hierarchy point is negative")
	}
	if fixed > math.MaxInt64-variable {
		return 0, fmt.Errorf("hierarchy point overflows")
	}
	return fixed + variable, nil
}

// ResolveHierarchyDetailGrade returns the strongest grade whose point and
// rank requirements are both satisfied.
func ResolveHierarchyDetailGrade(
	grades []*pmaster.HierarchyDetailGrade,
	point int64,
	rank int32,
) (*pmaster.HierarchyDetailGrade, error) {
	if point < 0 || rank <= 0 {
		return nil, fmt.Errorf("hierarchy point or rank is invalid")
	}
	ordered, err := orderedHierarchyDetailGrades(grades)
	if err != nil {
		return nil, err
	}
	for _, grade := range ordered {
		if point < grade.GetRequiredHierarchyPoint() {
			continue
		}
		if requiredRank := grade.GetRequiredHierarchyRank(); requiredRank > 0 && rank > requiredRank {
			continue
		}
		return grade, nil
	}
	return nil, fmt.Errorf("no hierarchy detail grade matches point %d and rank %d", point, rank)
}

// HierarchyDetailGradeReached reports whether achievedID is the same strength
// as targetID or stronger.
func HierarchyDetailGradeReached(
	grades []*pmaster.HierarchyDetailGrade,
	achievedID string,
	targetID string,
) (bool, error) {
	if achievedID == "" || targetID == "" {
		return false, nil
	}
	ordered, err := orderedHierarchyDetailGrades(grades)
	if err != nil {
		return false, err
	}
	achievedIndex := -1
	targetIndex := -1
	for index, grade := range ordered {
		if grade.GetId() == achievedID {
			achievedIndex = index
		}
		if grade.GetId() == targetID {
			targetIndex = index
		}
	}
	if achievedIndex < 0 {
		return false, fmt.Errorf("unknown achieved hierarchy detail grade %q", achievedID)
	}
	if targetIndex < 0 {
		return false, fmt.Errorf("unknown target hierarchy detail grade %q", targetID)
	}
	return achievedIndex <= targetIndex, nil
}

// StrongestHierarchyDetailGradeID returns whichever supplied grade is
// stronger, allowing an empty historical grade for fresh users.
func StrongestHierarchyDetailGradeID(
	grades []*pmaster.HierarchyDetailGrade,
	firstID string,
	secondID string,
) (string, error) {
	if firstID == "" {
		return secondID, nil
	}
	if secondID == "" {
		return firstID, nil
	}
	firstIsStronger, err := HierarchyDetailGradeReached(grades, firstID, secondID)
	if err != nil {
		return "", err
	}
	if firstIsStronger {
		return firstID, nil
	}
	return secondID, nil
}

// LoadHierarchyOverview evaluates a user's current and best grade, along with
// every currently reachable reward, against live master data.
func LoadHierarchyOverview(
	ctx context.Context,
	suid string,
	rank int32,
) (*HierarchyOverview, error) {
	hierarchy, err := opuser.FindUserHierarchy(ctx, suid)
	if err != nil {
		return nil, err
	}
	point, err := HierarchyTotalPoint(hierarchy)
	if err != nil {
		return nil, err
	}
	grades, err := opmaster.FindAllHierarchyDetailGrades(ctx)
	if err != nil {
		return nil, err
	}
	currentGrade, err := ResolveHierarchyDetailGrade(grades, point, rank)
	if err != nil {
		return nil, err
	}
	if hierarchy.GetBestTotalPoint() < 0 {
		return nil, FailedPreconditionf("Hierarchy best point is invalid")
	}
	bestPoint := max(hierarchy.GetBestTotalPoint(), point)
	bestPointGrade, err := ResolveHierarchyDetailGrade(grades, bestPoint, rank)
	if err != nil {
		return nil, err
	}
	achievedGradeID, err := StrongestHierarchyDetailGradeID(
		grades,
		bestPointGrade.GetId(),
		hierarchy.GetBestDetailGradeId(),
	)
	if err != nil {
		return nil, err
	}
	hierarchyChanged := hierarchy.GetBestTotalPoint() != bestPoint ||
		hierarchy.GetBestDetailGradeId() != achievedGradeID
	hierarchy.BestTotalPoint = bestPoint
	hierarchy.BestDetailGradeId = achievedGradeID

	rewards, err := opmaster.FindAllHierarchyRewards(ctx)
	if err != nil {
		return nil, err
	}
	rewardReached := make(map[string]bool, len(rewards))
	for _, reward := range rewards {
		targetGradeID, err := hierarchyRewardTargetGradeID(ctx, reward)
		if err != nil {
			return nil, err
		}
		rewardReached[reward.GetId()], err = HierarchyDetailGradeReached(
			grades,
			achievedGradeID,
			targetGradeID,
		)
		if err != nil {
			return nil, err
		}
	}

	divisionRewards, err := opmaster.FindAllHierarchyDivisionRewards(ctx)
	if err != nil {
		return nil, err
	}
	divisionRewardReached := make(map[string]bool, len(divisionRewards))
	for _, reward := range divisionRewards {
		if reward.GetDetailGradeId() == "" {
			return nil, FailedPreconditionf(
				"Hierarchy division reward %q has no detail grade",
				reward.GetId(),
			)
		}
		divisionRewardReached[reward.GetId()], err = HierarchyDetailGradeReached(
			grades,
			achievedGradeID,
			reward.GetDetailGradeId(),
		)
		if err != nil {
			return nil, err
		}
	}

	return &HierarchyOverview{
		Hierarchy:             hierarchy,
		HierarchyChanged:      hierarchyChanged,
		CurrentGrade:          currentGrade,
		CurrentRank:           rank,
		CurrentPoint:          point,
		Rewards:               rewards,
		DivisionRewards:       divisionRewards,
		RewardReached:         rewardReached,
		DivisionRewardReached: divisionRewardReached,
	}, nil
}

// PersistHierarchyNotification stores the current private-server rank and all
// reachable, unclaimed rewards for Home/Enter.
func PersistHierarchyNotification(
	ctx context.Context,
	suid string,
	overview *HierarchyOverview,
) (*papi.HierarchyNotiInfo, error) {
	if overview == nil || overview.Hierarchy == nil || overview.CurrentGrade == nil || overview.CurrentRank <= 0 {
		return nil, FailedPreconditionf("Hierarchy overview is missing")
	}
	receivedRewards := hierarchyStringSet(overview.Hierarchy.GetReceivedRewardIds())
	rewardIDs := make([]string, 0)
	for _, reward := range overview.Rewards {
		if _, received := receivedRewards[reward.GetId()]; overview.RewardReached[reward.GetId()] && !received {
			rewardIDs = append(rewardIDs, reward.GetId())
		}
	}
	receivedDivisionRewards := hierarchyStringSet(overview.Hierarchy.GetReceivedDivisionRewardIds())
	divisionRewardIDs := make([]string, 0)
	for _, reward := range overview.DivisionRewards {
		if _, received := receivedDivisionRewards[reward.GetId()]; overview.DivisionRewardReached[reward.GetId()] && !received {
			divisionRewardIDs = append(divisionRewardIDs, reward.GetId())
		}
	}
	info := &papi.HierarchyNotiInfo{
		RewardIds:            rewardIDs,
		DivisionRewardIds:    divisionRewardIDs,
		CurrentDetailGradeId: overview.CurrentGrade.GetId(),
		CurrentRank:          overview.CurrentRank,
	}
	if err := opuser.UpsertUserHierarchyNotiInfo(ctx, suid, info); err != nil {
		return nil, err
	}
	return info, nil
}

// ReconcileHierarchyNotification refreshes the hierarchy high-water state and
// Home notification in one transaction-friendly operation.
func ReconcileHierarchyNotification(
	ctx context.Context,
	suid string,
	rank int32,
) (*papi.HierarchyNotiInfo, *ptransaction.UserHierarchy, bool, error) {
	overview, err := LoadHierarchyOverview(ctx, suid, rank)
	if err != nil {
		return nil, nil, false, err
	}
	if overview.HierarchyChanged {
		if err := opuser.UpsertUserHierarchy(ctx, suid, overview.Hierarchy); err != nil {
			return nil, nil, false, err
		}
	}
	info, err := PersistHierarchyNotification(ctx, suid, overview)
	if err != nil {
		return nil, nil, false, err
	}
	return info, overview.Hierarchy, overview.HierarchyChanged, nil
}

func hierarchyRewardTargetGradeID(ctx context.Context, reward *pmaster.HierarchyReward) (string, error) {
	if reward == nil || reward.GetId() == "" || reward.GetConditionId() == "" {
		return "", FailedPreconditionf("Hierarchy reward master data is invalid")
	}
	condition, err := opmaster.FindCondition(ctx, reward.GetConditionId())
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", FailedPreconditionf("Hierarchy reward %q condition is missing", reward.GetId())
		}
		return "", err
	}
	targetGradeID := ""
	for _, setting := range condition.GetSettings() {
		if setting.GetHierarchyDetailGradeId() == nil {
			continue
		}
		candidate := setting.GetHierarchyDetailGradeId().GetHierarchyDetailGradeId()
		if candidate == "" || targetGradeID != "" {
			return "", FailedPreconditionf("Hierarchy reward %q condition is invalid", reward.GetId())
		}
		targetGradeID = candidate
	}
	if targetGradeID == "" {
		return "", FailedPreconditionf("Hierarchy reward %q has no detail-grade condition", reward.GetId())
	}
	return targetGradeID, nil
}

func hierarchyStringSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func orderedHierarchyDetailGrades(
	grades []*pmaster.HierarchyDetailGrade,
) ([]*pmaster.HierarchyDetailGrade, error) {
	if len(grades) == 0 {
		return nil, fmt.Errorf("hierarchy detail grade master data is empty")
	}
	ordered := append([]*pmaster.HierarchyDetailGrade(nil), grades...)
	for _, grade := range ordered {
		if grade == nil || grade.GetId() == "" || grade.GetRequiredHierarchyPoint() < 0 || grade.GetRequiredHierarchyRank() < 0 {
			return nil, fmt.Errorf("hierarchy detail grade master data is invalid")
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].GetRequiredHierarchyPoint() != ordered[j].GetRequiredHierarchyPoint() {
			return ordered[i].GetRequiredHierarchyPoint() > ordered[j].GetRequiredHierarchyPoint()
		}
		leftRank := ordered[i].GetRequiredHierarchyRank()
		rightRank := ordered[j].GetRequiredHierarchyRank()
		if leftRank == 0 {
			leftRank = math.MaxInt32
		}
		if rightRank == 0 {
			rightRank = math.MaxInt32
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return ordered[i].GetId() < ordered[j].GetId()
	})
	return ordered, nil
}

const HierarchyPrivateServerRank int32 = 1
