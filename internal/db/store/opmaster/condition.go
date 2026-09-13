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

func FindCondition(ctx context.Context, id string) (*pmaster.Condition, error) {
	return mcommon.FindOne[mmodel.Condition, pmaster.Condition](
		ctx, mname.Condition, bson.M{"Id": id},
	)
}

func FindConditions(ctx context.Context, ids []string) ([]*pmaster.Condition, error) {
	if len(ids) == 0 {
		return []*pmaster.Condition{}, nil
	}
	return mcommon.FindMany[mmodel.Condition, pmaster.Condition](
		ctx, mname.Condition, bson.M{"Id": bson.M{"$in": ids}},
	)
}

func FindAllConditions(ctx context.Context) ([]*pmaster.Condition, error) {
	return mcommon.FindMany[mmodel.Condition, pmaster.Condition](
		ctx,
		mname.Condition,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}),
	)
}

func FindFunctionLock(ctx context.Context, lockType penum.FunctionLockType) (*pmaster.FunctionLock, error) {
	return mcommon.FindOne[mmodel.FunctionLock, pmaster.FunctionLock](
		ctx, mname.FunctionLock, bson.M{"Type": int64(lockType)},
	)
}

func FindArea(ctx context.Context, id string) (*pmaster.Area, error) {
	return mcommon.FindOne[mmodel.Area, pmaster.Area](
		ctx, mname.Area, bson.M{"Id": id},
	)
}

func FindItem(ctx context.Context, id string) (*pmaster.Item, error) {
	return mcommon.FindOne[mmodel.Item, pmaster.Item](ctx, mname.Item, bson.M{"Id": id})
}

func FindPhotoRecipe(ctx context.Context, id string) (*pmaster.PhotoRecipe, error) {
	return mcommon.FindOne[mmodel.PhotoRecipe, pmaster.PhotoRecipe](ctx, mname.PhotoRecipe, bson.M{"Id": id})
}

func FindPhotoPose(ctx context.Context, id string) (*pmaster.PhotoPose, error) {
	return mcommon.FindOne[mmodel.PhotoPose, pmaster.PhotoPose](ctx, mname.PhotoPose, bson.M{"Id": id})
}

func FindShowcaseToy(ctx context.Context, id string) (*pmaster.ShowcaseToy, error) {
	return mcommon.FindOne[mmodel.ShowcaseToy, pmaster.ShowcaseToy](ctx, mname.ShowcaseToy, bson.M{"Id": id})
}
