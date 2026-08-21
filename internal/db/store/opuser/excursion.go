package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindUserExcursionState(ctx context.Context, suid string) (*umodel.UserExcursionState, error) {
	return ucommon.FindOne[umodel.UserExcursionState](ctx, uname.UserExcursionState, bson.M{"SUID": suid})
}

func UpsertUserExcursionState(ctx context.Context, state *umodel.UserExcursionState) error {
	return ucommon.UpsertOne(ctx, uname.UserExcursionState, bson.M{"SUID": state.SUID}, state)
}

func FindAllUserExcursionGazeReactions(ctx context.Context, suid string) ([]*ptransaction.UserExcursionGazeReaction, error) {
	return ucommon.FindAll[ptransaction.UserExcursionGazeReaction](
		ctx,
		uname.UserExcursionGazeReaction,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{
			{Key: "characterId", Value: 1},
			{Key: "excursionPlaceId", Value: 1},
			{Key: "gazeReactionNumber", Value: 1},
		}),
	)
}

func FindUserExcursionGazeReaction(
	ctx context.Context,
	suid, characterId, placeId string,
	number int64,
) (*ptransaction.UserExcursionGazeReaction, error) {
	return ucommon.FindOne[ptransaction.UserExcursionGazeReaction](
		ctx,
		uname.UserExcursionGazeReaction,
		bson.M{
			"SUID":               suid,
			"characterId":        characterId,
			"excursionPlaceId":   placeId,
			"gazeReactionNumber": number,
		},
	)
}

func UpsertUserExcursionGazeReaction(ctx context.Context, suid string, reaction *ptransaction.UserExcursionGazeReaction) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserExcursionGazeReaction,
		bson.M{
			"SUID":               suid,
			"characterId":        reaction.CharacterId,
			"excursionPlaceId":   reaction.ExcursionPlaceId,
			"gazeReactionNumber": reaction.GazeReactionNumber,
		},
		&umodel.UserStruct[*ptransaction.UserExcursionGazeReaction]{SUID: suid, Data: reaction},
	)
}
