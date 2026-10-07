package opmaster

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindExercise(ctx context.Context, id string) (*pmaster.Exercise, error) {
	return mcommon.FindOne[mmodel.Exercise, pmaster.Exercise](ctx, mname.Exercise, bson.M{"Id": id})
}

func FindAllExercises(ctx context.Context) ([]*pmaster.Exercise, error) {
	rows, err := mcommon.FindMany[mmodel.Exercise, pmaster.Exercise](ctx, mname.Exercise, bson.M{})
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].GetId() < rows[j].GetId() })
	return rows, nil
}

func FindExerciseUser(ctx context.Context, id string) (*pmaster.ExerciseUser, error) {
	return mcommon.FindOne[mmodel.ExerciseUser, pmaster.ExerciseUser](ctx, mname.ExerciseUser, bson.M{"Id": id})
}

func FindExerciseDeck(ctx context.Context, id string) (*pmaster.ExerciseDeck, error) {
	return mcommon.FindOne[mmodel.ExerciseDeck, pmaster.ExerciseDeck](ctx, mname.ExerciseDeck, bson.M{"Id": id})
}

func FindExerciseCardGroup(ctx context.Context, id string) (*pmaster.ExerciseCardGroup, error) {
	return mcommon.FindOne[mmodel.ExerciseCardGroup, pmaster.ExerciseCardGroup](ctx, mname.ExerciseCardGroup, bson.M{"GroupId": id})
}

func FindExercisePhotoGroup(ctx context.Context, id string) (*pmaster.ExercisePhotoGroup, error) {
	return mcommon.FindOne[mmodel.ExercisePhotoGroup, pmaster.ExercisePhotoGroup](ctx, mname.ExercisePhotoGroup, bson.M{"GroupId": id})
}

func FindExerciseAccessoryGroup(ctx context.Context, id string) (*pmaster.ExerciseAccessoryGroup, error) {
	return mcommon.FindOne[mmodel.ExerciseAccessoryGroup, pmaster.ExerciseAccessoryGroup](ctx, mname.ExerciseAccessoryGroup, bson.M{"GroupId": id})
}

func FindAllExerciseCostumes(ctx context.Context) ([]*pmaster.ExerciseCostume, error) {
	rows, err := mcommon.FindMany[mmodel.ExerciseCostume, pmaster.ExerciseCostume](ctx, mname.ExerciseCostume, bson.M{})
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].GetCharacterId() < rows[j].GetCharacterId() })
	return rows, nil
}
