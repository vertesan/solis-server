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

func FindPhotoAbility(ctx context.Context, id string) (*pmaster.PhotoAbility, error) {
	return mcommon.FindOne[mmodel.PhotoAbility, pmaster.PhotoAbility](
		ctx, mname.PhotoAbility, bson.M{"Id": id},
	)
}

func FindPhotoAbilities(ctx context.Context, ids []string) ([]*pmaster.PhotoAbility, error) {
	if len(ids) == 0 {
		return []*pmaster.PhotoAbility{}, nil
	}
	return mcommon.FindMany[mmodel.PhotoAbility, pmaster.PhotoAbility](
		ctx, mname.PhotoAbility, bson.M{"Id": bson.M{"$in": ids}},
	)
}

func FindPhotoAbilitySet(ctx context.Context, id string) (*pmaster.PhotoAbilitySet, error) {
	return mcommon.FindOne[mmodel.PhotoAbilitySet, pmaster.PhotoAbilitySet](
		ctx, mname.PhotoAbilitySet, bson.M{"Id": id},
	)
}

func FindAllPhotoAbilitySets(ctx context.Context) ([]*pmaster.PhotoAbilitySet, error) {
	return mcommon.FindMany[mmodel.PhotoAbilitySet, pmaster.PhotoAbilitySet](
		ctx, mname.PhotoAbilitySet, bson.M{},
	)
}

func FindPhotoRarityLevel(ctx context.Context, rarity, level int32) (*pmaster.PhotoRarityLevel, error) {
	return mcommon.FindOne[mmodel.PhotoRarityLevel, pmaster.PhotoRarityLevel](
		ctx,
		mname.PhotoRarityLevel,
		bson.M{"Rarity": rarity, "Level": level},
	)
}

func FindPhotoRarityLevels(ctx context.Context, rarity int32) ([]*pmaster.PhotoRarityLevel, error) {
	return mcommon.FindMany[mmodel.PhotoRarityLevel, pmaster.PhotoRarityLevel](
		ctx,
		mname.PhotoRarityLevel,
		bson.M{"Rarity": rarity},
		options.Find().SetSort(bson.D{{Key: "Level", Value: 1}}),
	)
}

func FindAllPhotoRarityLevels(ctx context.Context) ([]*pmaster.PhotoRarityLevel, error) {
	return mcommon.FindMany[mmodel.PhotoRarityLevel, pmaster.PhotoRarityLevel](
		ctx,
		mname.PhotoRarityLevel,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Rarity", Value: 1}, {Key: "Level", Value: 1}}),
	)
}

func FindPhotoRetouchEnhance(ctx context.Context, groupID string, photoLevel int32) (*pmaster.PhotoRetouchEnhance, error) {
	return mcommon.FindOne[mmodel.PhotoRetouchEnhance, pmaster.PhotoRetouchEnhance](
		ctx,
		mname.PhotoRetouchEnhance,
		bson.M{"GroupId": groupID, "PhotoLevel": photoLevel},
	)
}

func FindPhotoActivity(ctx context.Context, id string) (*pmaster.PhotoActivity, error) {
	return mcommon.FindOne[mmodel.PhotoActivity, pmaster.PhotoActivity](
		ctx, mname.PhotoActivity, bson.M{"Id": id},
	)
}

func FindPhotoExpression(ctx context.Context, id string) (*pmaster.PhotoExpression, error) {
	return mcommon.FindOne[mmodel.PhotoExpression, pmaster.PhotoExpression](
		ctx, mname.PhotoExpression, bson.M{"Id": id},
	)
}

func FindPhotoQuestMusic(ctx context.Context, id string) (*pmaster.PhotoQuestMusic, error) {
	return mcommon.FindOne[mmodel.PhotoQuestMusic, pmaster.PhotoQuestMusic](
		ctx, mname.PhotoQuestMusic, bson.M{"Id": id},
	)
}

func FindAllPhotoQuestMusics(ctx context.Context) ([]*pmaster.PhotoQuestMusic, error) {
	return mcommon.FindMany[mmodel.PhotoQuestMusic, pmaster.PhotoQuestMusic](
		ctx,
		mname.PhotoQuestMusic,
		bson.M{},
	)
}

func FindPhotoQuestStage(ctx context.Context, id string) (*pmaster.PhotoQuestStage, error) {
	return mcommon.FindOne[mmodel.PhotoQuestStage, pmaster.PhotoQuestStage](
		ctx, mname.PhotoQuestStage, bson.M{"Id": id},
	)
}

func FindAllPhotoQuestStages(ctx context.Context) ([]*pmaster.PhotoQuestStage, error) {
	return mcommon.FindMany[mmodel.PhotoQuestStage, pmaster.PhotoQuestStage](
		ctx,
		mname.PhotoQuestStage,
		bson.M{},
	)
}

func FindPhotoExpressionStagePositions(ctx context.Context, stageID string) ([]*pmaster.PhotoExpressionStagePosition, error) {
	return mcommon.FindMany[mmodel.PhotoExpressionStagePosition, pmaster.PhotoExpressionStagePosition](
		ctx,
		mname.PhotoExpressionStagePosition,
		bson.M{"StageId": stageID},
		options.Find().SetSort(bson.D{{Key: "Number", Value: 1}}),
	)
}

func FindPhotoStage(ctx context.Context, id string) (*pmaster.Stage, error) {
	return mcommon.FindOne[mmodel.Stage, pmaster.Stage](
		ctx, mname.Stage, bson.M{"Id": id},
	)
}

func FindPhotoFacial(ctx context.Context, id string) (*pmaster.PhotoFacial, error) {
	return mcommon.FindOne[mmodel.PhotoFacial, pmaster.PhotoFacial](
		ctx, mname.PhotoFacial, bson.M{"Id": id},
	)
}
