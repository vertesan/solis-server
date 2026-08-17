package opuser

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
)

type GodUpsert[T proto.Message] struct {
	Key  bson.M
	Data T
}

func UpsertManyForGod[T proto.Message](
	ctx context.Context,
	collection string,
	suid string,
	values []GodUpsert[T],
	overwrite bool,
) error {
	if len(values) == 0 {
		return nil
	}

	models := make([]mongo.WriteModel, 0, len(values))
	for _, value := range values {
		filter := bson.M{"SUID": suid}
		for key, item := range value.Key {
			filter[key] = item
		}
		document := &umodel.UserStruct[T]{SUID: suid, Data: value.Data}
		operator := "$setOnInsert"
		if overwrite {
			operator = "$set"
		}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(bson.M{operator: document}).
			SetUpsert(true))
	}

	if _, err := depot.User.Collection(collection).BulkWrite(
		ctx,
		models,
		options.BulkWrite().SetOrdered(false),
	); err != nil {
		return fmt.Errorf("failed to upsert god data for collection %q: %w", collection, err)
	}
	return nil
}

func RaiseUserResourcesForGod(ctx context.Context, suid string, minimum int64) error {
	for _, collection := range []string{uname.UserItem, uname.UserAccessory, uname.UserPhotoRecipe} {
		if _, err := depot.User.Collection(collection).UpdateMany(
			ctx,
			bson.M{"SUID": suid, "amount": bson.M{"$lt": minimum}},
			bson.M{"$set": bson.M{"amount": minimum}},
		); err != nil {
			return fmt.Errorf("failed to raise god resources for collection %q: %w", collection, err)
		}
	}
	return nil
}

func RaiseUserPointsForGod(ctx context.Context, suid string, minimum int64) error {
	if _, err := depot.User.Collection(uname.UserPoint).UpdateMany(
		ctx,
		bson.M{"SUID": suid, "amount": bson.M{"$lt": minimum}},
		bson.M{"$set": bson.M{"amount": minimum}},
	); err != nil {
		return fmt.Errorf("failed to raise god points for collection %q: %w", uname.UserPoint, err)
	}
	return nil
}

func RaiseUserBalanceForGod(ctx context.Context, suid string, minimum int32) error {
	if _, err := depot.User.Collection(uname.UserBalance).UpdateOne(
		ctx,
		bson.M{"SUID": suid},
		bson.M{
			"$setOnInsert": bson.M{"SUID": suid},
			"$max": bson.M{
				"freeBalance": minimum,
				"paidBalance": minimum,
			},
		},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return fmt.Errorf("failed to raise god resources for collection %q: %w", uname.UserBalance, err)
	}
	return nil
}

func UnlockUserAreasForGod(
	ctx context.Context,
	suid string,
	areas []*ptransaction.UserArea,
) error {
	models := make([]mongo.WriteModel, 0, len(areas))
	for _, area := range areas {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"SUID": suid, "areaId": area.AreaId}).
			SetUpdate(bson.M{
				"$setOnInsert": bson.M{"SUID": suid, "areaId": area.AreaId},
				"$max": bson.M{
					"clearQuestCount": area.ClearQuestCount,
					"lastClearedTime": area.LastClearedTime,
				},
			}).
			SetUpsert(true))
	}
	return bulkWriteGodData(ctx, uname.UserArea, models)
}

func UnlockUserQuestsForGod(
	ctx context.Context,
	suid string,
	quests []*ptransaction.UserQuest,
) error {
	models := make([]mongo.WriteModel, 0, len(quests))
	for _, quest := range quests {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"SUID": suid, "questId": quest.QuestId}).
			SetUpdate(bson.M{
				"$setOnInsert": bson.M{"SUID": suid, "questId": quest.QuestId},
				"$max": bson.M{
					"lastClearedTime":  quest.LastClearedTime,
					"highestScore":     quest.HighestScore,
					"highestRank":      quest.HighestRank,
					"highestScoreRank": quest.HighestScoreRank,
				},
			}).
			SetUpsert(true))
	}
	return bulkWriteGodData(ctx, uname.UserQuest, models)
}

func bulkWriteGodData(ctx context.Context, collection string, models []mongo.WriteModel) error {
	if len(models) == 0 {
		return nil
	}
	if _, err := depot.User.Collection(collection).BulkWrite(
		ctx,
		models,
		options.BulkWrite().SetOrdered(false),
	); err != nil {
		return fmt.Errorf("failed to write god data for collection %q: %w", collection, err)
	}
	return nil
}
