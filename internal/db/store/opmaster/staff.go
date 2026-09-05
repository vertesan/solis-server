package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindStaffTraining(ctx context.Context, parameterType penum.ParameterType) (*pmaster.StaffTraining, error) {
	return mcommon.FindOne[mmodel.StaffTraining, pmaster.StaffTraining](
		ctx,
		mname.StaffTraining,
		bson.M{"ParameterType": parameterType},
	)
}

func FindAllStaffTrainings(ctx context.Context) ([]*pmaster.StaffTraining, error) {
	return mcommon.FindMany[mmodel.StaffTraining, pmaster.StaffTraining](
		ctx,
		mname.StaffTraining,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "ParameterType", Value: 1}}),
	)
}

func FindStaffLevels(ctx context.Context, parameterType penum.ParameterType) ([]*pmaster.StaffLevel, error) {
	return mcommon.FindMany[mmodel.StaffLevel, pmaster.StaffLevel](
		ctx,
		mname.StaffLevel,
		bson.M{"ParameterType": parameterType},
		options.Find().SetSort(bson.D{{Key: "Level", Value: 1}}),
	)
}

func FindAllStaffLevels(ctx context.Context) ([]*pmaster.StaffLevel, error) {
	return mcommon.FindMany[mmodel.StaffLevel, pmaster.StaffLevel](
		ctx,
		mname.StaffLevel,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "ParameterType", Value: 1}, {Key: "Level", Value: 1}}),
	)
}

func FindStaffLevelLimitBreakRank(
	ctx context.Context,
	parameterType penum.ParameterType,
	rank int32,
) (*pmaster.StaffLevelLimitBreakRank, error) {
	return mcommon.FindOne[mmodel.StaffLevelLimitBreakRank, pmaster.StaffLevelLimitBreakRank](
		ctx,
		mname.StaffLevelLimitBreakRank,
		bson.M{"ParameterType": parameterType, "Rank": rank},
	)
}

func FindAllStaffLevelLimitBreakRanks(ctx context.Context) ([]*pmaster.StaffLevelLimitBreakRank, error) {
	return mcommon.FindMany[mmodel.StaffLevelLimitBreakRank, pmaster.StaffLevelLimitBreakRank](
		ctx,
		mname.StaffLevelLimitBreakRank,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "ParameterType", Value: 1}, {Key: "Rank", Value: 1}}),
	)
}

func FindTotalStaffEnhanceAdvantage(
	ctx context.Context,
	enhanceCount int32,
) (*pmaster.TotalStaffEnhanceAdvantage, error) {
	return mcommon.FindOne[mmodel.TotalStaffEnhanceAdvantage, pmaster.TotalStaffEnhanceAdvantage](
		ctx,
		mname.TotalStaffEnhanceAdvantage,
		bson.M{"EnhanceCount": enhanceCount},
	)
}
