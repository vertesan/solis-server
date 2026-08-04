package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindUserDeck(ctx context.Context, suid string, number int32) (*ptransaction.UserDeck, error) {
	return ucommon.FindOne[ptransaction.UserDeck](
		ctx,
		uname.UserDeck,
		bson.M{"SUID": suid, "number": number},
	)
}

func FindAllUserDecks(ctx context.Context, suid string) ([]*ptransaction.UserDeck, error) {
	return ucommon.FindAll[ptransaction.UserDeck](
		ctx,
		uname.UserDeck,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "number", Value: 1}}),
	)
}

func UpsertUserDeck(ctx context.Context, suid string, deck *ptransaction.UserDeck) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserDeck,
		bson.M{
			"SUID":   suid,
			"number": deck.Number,
		},
		&ustruct.UserStruct[*ptransaction.UserDeck]{
			SUID: suid,
			Data: deck,
		},
	)
}

// Note: This function is preferred to be used inside a transaction if there are more than 1 objects in the slice
func UpsertManyUserDecks(ctx context.Context, suid string, decks []*ptransaction.UserDeck) error {
	for _, deck := range decks {
		if err := UpsertUserDeck(ctx, suid, deck); err != nil {
			return err
		}
	}
	return nil
}

func DeleteUserDeck(ctx context.Context, suid string, number int32) error {
	return ucommon.Delete(
		ctx,
		uname.UserDeck,
		bson.M{"SUID": suid, "number": number},
	)
}

func DeleteAllUserDecks(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserDeck,
		bson.M{"SUID": suid},
	)
}
