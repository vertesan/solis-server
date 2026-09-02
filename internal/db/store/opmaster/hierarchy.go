package opmaster

import (
	"context"
	"math"
	"sort"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindHierarchyDetailGrade(ctx context.Context, id string) (*pmaster.HierarchyDetailGrade, error) {
	return mcommon.FindOne[mmodel.HierarchyDetailGrade, pmaster.HierarchyDetailGrade](
		ctx, mname.HierarchyDetailGrade, bson.M{"Id": id},
	)
}

func FindAllHierarchyDetailGrades(ctx context.Context) ([]*pmaster.HierarchyDetailGrade, error) {
	grades, err := mcommon.FindMany[mmodel.HierarchyDetailGrade, pmaster.HierarchyDetailGrade](
		ctx,
		mname.HierarchyDetailGrade,
		bson.M{},
	)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(grades, func(i, j int) bool {
		if grades[i].GetRequiredHierarchyPoint() != grades[j].GetRequiredHierarchyPoint() {
			return grades[i].GetRequiredHierarchyPoint() > grades[j].GetRequiredHierarchyPoint()
		}
		leftRank := grades[i].GetRequiredHierarchyRank()
		rightRank := grades[j].GetRequiredHierarchyRank()
		if leftRank == 0 {
			leftRank = math.MaxInt32
		}
		if rightRank == 0 {
			rightRank = math.MaxInt32
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return grades[i].GetId() < grades[j].GetId()
	})
	return grades, nil
}

func FindHierarchyReward(ctx context.Context, id string) (*pmaster.HierarchyReward, error) {
	return mcommon.FindOne[mmodel.HierarchyReward, pmaster.HierarchyReward](
		ctx, mname.HierarchyReward, bson.M{"Id": id},
	)
}

func FindAllHierarchyRewards(ctx context.Context) ([]*pmaster.HierarchyReward, error) {
	rewards, err := mcommon.FindMany[mmodel.HierarchyReward, pmaster.HierarchyReward](
		ctx,
		mname.HierarchyReward,
		bson.M{},
	)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rewards, func(i, j int) bool {
		if rewards[i].GetOrder() != rewards[j].GetOrder() {
			return rewards[i].GetOrder() < rewards[j].GetOrder()
		}
		return rewards[i].GetId() < rewards[j].GetId()
	})
	return rewards, nil
}

func FindHierarchyDivisionReward(ctx context.Context, id string) (*pmaster.HierarchyDivisionReward, error) {
	return mcommon.FindOne[mmodel.HierarchyDivisionReward, pmaster.HierarchyDivisionReward](
		ctx, mname.HierarchyDivisionReward, bson.M{"Id": id},
	)
}

func FindAllHierarchyDivisionRewards(ctx context.Context) ([]*pmaster.HierarchyDivisionReward, error) {
	rewards, err := mcommon.FindMany[mmodel.HierarchyDivisionReward, pmaster.HierarchyDivisionReward](
		ctx,
		mname.HierarchyDivisionReward,
		bson.M{},
	)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rewards, func(i, j int) bool {
		if rewards[i].GetOrder() != rewards[j].GetOrder() {
			return rewards[i].GetOrder() > rewards[j].GetOrder()
		}
		return rewards[i].GetId() < rewards[j].GetId()
	})
	return rewards, nil
}
