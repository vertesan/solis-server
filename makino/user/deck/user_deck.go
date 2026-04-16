package deck

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"vertesan/solis-server/db/crud/opuser"
	"vertesan/solis-server/proto/ptransaction"
)

func SpawnUserDeck(suid string, number int32, name string) (*ptransaction.UserDeck, error) {
	if number != 0 {
		return &ptransaction.UserDeck{
			Number: number,
			Name:   name,
		}, nil
	}

	decks, err := opuser.FindAllUserDecks(context.Background(), suid)
	if err != nil {
		return nil, err
	}
	var num int32 = 1
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
