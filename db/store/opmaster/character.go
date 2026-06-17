package opmaster

import (
	"context"
	"iter"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/db/store/opmaster/internal/mcommon"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindCharacter(ctx context.Context, id string) (*pmaster.Character, error) {
	return mcommon.FindOne[mstruct.Character, pmaster.Character](
		ctx, mname.Character, bson.M{"Id": id},
	)
}

func FindAllCharacters(ctx context.Context) ([]*pmaster.Character, error) {
	return mcommon.FindMany[mstruct.Character, pmaster.Character](
		ctx, mname.Character, bson.M{},
	)
}

func YieldAllCharacters(ctx context.Context) iter.Seq[*pmaster.Character] {
	return func(yield func(*pmaster.Character) bool) {
		for c := range mcommon.YieldMany[mstruct.Character, pmaster.Character](ctx, mname.Character, bson.M{}) {
			if !yield(c) {
				return
			}
		}
	}
}

func FindCharacterActivityLevel(ctx context.Context, level int64) (*pmaster.CharacterActivityLevel, error) {
	return mcommon.FindOne[mstruct.CharacterActivityLevel, pmaster.CharacterActivityLevel](
		ctx, mname.CharacterActivityLevel, bson.M{"Level": level},
	)
}
