package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserExerciseDeck(ctx context.Context, suid, id string) (*umodel.UserExerciseDeck, error) {
	return ucommon.FindOne[umodel.UserExerciseDeck](ctx, uname.UserExerciseDeck, bson.M{"_id": suid + ":" + id, "SUID": suid})
}

func FindAllUserExerciseDecks(ctx context.Context, suid string) ([]*umodel.UserExerciseDeck, error) {
	return ucommon.FindAll[umodel.UserExerciseDeck](ctx, uname.UserExerciseDeck, bson.M{"SUID": suid})
}

func UpsertUserExerciseDeck(ctx context.Context, deck *umodel.UserExerciseDeck) error {
	return ucommon.UpsertOne(ctx, uname.UserExerciseDeck, bson.M{"_id": deck.SUID + ":" + deck.ExerciseID, "SUID": deck.SUID}, deck)
}
