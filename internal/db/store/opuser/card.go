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
		&umodel.UserStruct[*ptransaction.UserCard]{
			SUID: suid,
			Data: card,
		},
	)
}

func UpsertManyUserCards(ctx context.Context, suid string, cards []*ptransaction.UserCard) error {
	return ucommon.ForEachAtomically(ctx, cards, func(ctx context.Context, card *ptransaction.UserCard) error {
		return UpsertUserCard(ctx, suid, card)
	})
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
