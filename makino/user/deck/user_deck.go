package deck

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"vertesan/solis-server/db/store/opuser"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func SpawnUserDeck(suid string, number int32, name string) (*ptransaction.UserDeck, error) {
	if number != 0 {
		return &ptransaction.UserDeck{
			Number: number,
			Name:   name,
		}, nil
	}
	var num int32 = 1
	decks, err := opuser.FindAllUserDecks(context.Background(), suid)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return &ptransaction.UserDeck{
				Number: num,
				Name:   fmt.Sprintf("ユニット%d", num),
			}, nil
		}
		return nil, err
	}

	if len(decks) > 1 {
		maxDeck := slices.MaxFunc(decks, func(a, b *ptransaction.UserDeck) int {
			return cmp.Compare(a.Number, b.Number)
		})
		num = maxDeck.Number + 1
	}
	return &ptransaction.UserDeck{
		Number: num,
		Name:   fmt.Sprintf("ユニット%d", num),
	}, nil
}
