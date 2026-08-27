package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindCompanyGrade(ctx context.Context, id string) (*pmaster.CompanyGrade, error) {
	return mcommon.FindOne[mmodel.CompanyGrade, pmaster.CompanyGrade](
		ctx, mname.CompanyGrade, bson.M{"Id": id},
	)
}

func FindAllCompanyGrades(ctx context.Context) ([]*pmaster.CompanyGrade, error) {
	return mcommon.FindMany[mmodel.CompanyGrade, pmaster.CompanyGrade](
		ctx,
		mname.CompanyGrade,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "RequiredPoint", Value: 1}, {Key: "Id", Value: 1}}),
	)
}

func FindCompanyTrustLevel(ctx context.Context, levelKey int32) (*pmaster.CompanyTrustLevel, error) {
	return mcommon.FindOne[mmodel.CompanyTrustLevel, pmaster.CompanyTrustLevel](
		ctx, mname.CompanyTrustLevel, bson.M{"LevelKey": levelKey},
	)
}

func FindAllCompanyTrustLevels(ctx context.Context) ([]*pmaster.CompanyTrustLevel, error) {
	return mcommon.FindMany[mmodel.CompanyTrustLevel, pmaster.CompanyTrustLevel](
		ctx,
		mname.CompanyTrustLevel,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "LevelKey", Value: 1}}),
	)
}

func FindCompanyEnjoyLevel(ctx context.Context, levelKey int32) (*pmaster.CompanyEnjoyLevel, error) {
	return mcommon.FindOne[mmodel.CompanyEnjoyLevel, pmaster.CompanyEnjoyLevel](
		ctx, mname.CompanyEnjoyLevel, bson.M{"LevelKey": levelKey},
	)
}

func FindAllCompanyEnjoyLevels(ctx context.Context) ([]*pmaster.CompanyEnjoyLevel, error) {
	return mcommon.FindMany[mmodel.CompanyEnjoyLevel, pmaster.CompanyEnjoyLevel](
		ctx,
		mname.CompanyEnjoyLevel,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "LevelKey", Value: 1}}),
	)
}

func FindCompanyEnjoyCharacterLevel(
	ctx context.Context,
	levelKey int32,
) (*pmaster.CompanyEnjoyCharacterLevel, error) {
	return mcommon.FindOne[mmodel.CompanyEnjoyCharacterLevel, pmaster.CompanyEnjoyCharacterLevel](
		ctx, mname.CompanyEnjoyCharacterLevel, bson.M{"LevelKey": levelKey},
	)
}

func FindAllCompanyEnjoyCharacterLevels(ctx context.Context) ([]*pmaster.CompanyEnjoyCharacterLevel, error) {
	return mcommon.FindMany[mmodel.CompanyEnjoyCharacterLevel, pmaster.CompanyEnjoyCharacterLevel](
		ctx,
		mname.CompanyEnjoyCharacterLevel,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "LevelKey", Value: 1}}),
	)
}

func FindCompanyEnjoyCharacterLevelSetting(
	ctx context.Context,
	characterId string,
	levelKey int32,
) (*pmaster.CompanyEnjoyCharacterLevelSetting, error) {
	return mcommon.FindOne[mmodel.CompanyEnjoyCharacterLevelSetting, pmaster.CompanyEnjoyCharacterLevelSetting](
		ctx,
		mname.CompanyEnjoyCharacterLevelSetting,
		bson.M{"CharacterId": characterId, "LevelKey": levelKey},
	)
}

func FindCompanyEnjoyCharacterLevelSettings(
	ctx context.Context,
	characterId string,
) ([]*pmaster.CompanyEnjoyCharacterLevelSetting, error) {
	return mcommon.FindMany[mmodel.CompanyEnjoyCharacterLevelSetting, pmaster.CompanyEnjoyCharacterLevelSetting](
		ctx,
		mname.CompanyEnjoyCharacterLevelSetting,
		bson.M{"CharacterId": characterId},
		options.Find().SetSort(bson.D{{Key: "LevelKey", Value: 1}}),
	)
}

func FindAllCompanyEnjoyCharacterLevelSettings(
	ctx context.Context,
) ([]*pmaster.CompanyEnjoyCharacterLevelSetting, error) {
	return mcommon.FindMany[mmodel.CompanyEnjoyCharacterLevelSetting, pmaster.CompanyEnjoyCharacterLevelSetting](
		ctx,
		mname.CompanyEnjoyCharacterLevelSetting,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "CharacterId", Value: 1}, {Key: "LevelKey", Value: 1}}),
	)
}

func FindCompanyEnjoyObtainPointItemSetting(
	ctx context.Context,
	characterId string,
	itemId string,
) (*pmaster.CompanyEnjoyObtainPointItemSetting, error) {
	return mcommon.FindOne[mmodel.CompanyEnjoyObtainPointItemSetting, pmaster.CompanyEnjoyObtainPointItemSetting](
		ctx,
		mname.CompanyEnjoyObtainPointItemSetting,
		bson.M{"CharacterId": characterId, "ItemId": itemId},
	)
}

func FindCompanyEnjoyObtainPointItemSettings(
	ctx context.Context,
	characterId string,
) ([]*pmaster.CompanyEnjoyObtainPointItemSetting, error) {
	return mcommon.FindMany[mmodel.CompanyEnjoyObtainPointItemSetting, pmaster.CompanyEnjoyObtainPointItemSetting](
		ctx,
		mname.CompanyEnjoyObtainPointItemSetting,
		bson.M{"CharacterId": characterId},
		options.Find().SetSort(bson.D{{Key: "ItemId", Value: 1}}),
	)
}

func FindAllCompanyEnjoyObtainPointItemSettings(
	ctx context.Context,
) ([]*pmaster.CompanyEnjoyObtainPointItemSetting, error) {
	return mcommon.FindMany[mmodel.CompanyEnjoyObtainPointItemSetting, pmaster.CompanyEnjoyObtainPointItemSetting](
		ctx,
		mname.CompanyEnjoyObtainPointItemSetting,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "CharacterId", Value: 1}, {Key: "ItemId", Value: 1}}),
	)
}

func FindCompanyPoint(ctx context.Context, id string) (*pmaster.CompanyPoint, error) {
	return mcommon.FindOne[mmodel.CompanyPoint, pmaster.CompanyPoint](
		ctx, mname.CompanyPoint, bson.M{"Id": id},
	)
}

func FindAllCompanyPoints(ctx context.Context) ([]*pmaster.CompanyPoint, error) {
	return mcommon.FindMany[mmodel.CompanyPoint, pmaster.CompanyPoint](
		ctx,
		mname.CompanyPoint,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Id", Value: 1}}),
	)
}
