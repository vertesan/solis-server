package deck

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func SpawnUserDeck(ctx context.Context, suid string, number int32, name string) (*ptransaction.UserDeck, error) {
	if number != 0 {
		return &ptransaction.UserDeck{
			Number: number,
			Name:   name,
		}, nil
	}
	user, err := opuser.FindAndUpdateUser(
		ctx,
		suid,
		bson.M{"$inc": bson.M{"deckSequence": 1}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before),
	)
	if err != nil {
		return nil, err
	}

	num := int32(user.DeckSequence)
	return &ptransaction.UserDeck{
		Number: num,
		Name:   fmt.Sprintf("ユニット%d", num),
	}, nil
}
