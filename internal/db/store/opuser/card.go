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

func FindUserCard(ctx context.Context, suid string, cardId string) (*ptransaction.UserCard, error) {
	return ucommon.FindOne[ptransaction.UserCard](
		ctx,
		uname.UserCard,
		bson.M{"SUID": suid, "cardId": cardId},
	)
}

func FindAllUserCards(ctx context.Context, suid string) ([]*ptransaction.UserCard, error) {
	return ucommon.FindAll[ptransaction.UserCard](
		ctx,
		uname.UserCard,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "cardId", Value: 1}}),
	)
}

func UpsertUserCard(ctx context.Context, suid string, card *ptransaction.UserCard) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserCard,
		bson.M{
			"SUID":   suid,
			"cardId": card.CardId,
		},
		&ustruct.UserStruct[*ptransaction.UserCard]{
			SUID: suid,
			Data: card,
		},
	)
}

// Note: This function is preferred to be used inside a transaction if there are more than 1 objects in the slice
func UpsertManyUserCards(ctx context.Context, suid string, cards []*ptransaction.UserCard) error {
	for _, card := range cards {
		if err := UpsertUserCard(ctx, suid, card); err != nil {
			return err
		}
	}
	return nil
}

func DeleteUserCard(ctx context.Context, suid string, cardId string) error {
	return ucommon.Delete(
		ctx,
		uname.UserCard,
		bson.M{"SUID": suid, "cardId": cardId},
	)
}

func DeleteAllUserCards(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserCard,
		bson.M{"SUID": suid},
	)
}
