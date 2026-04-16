package opmaster

import (
	"context"
	"vertesan/solis-server/db/crud/opmaster/internal/mcommon"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindCardImages(ctx context.Context, cardId string) ([]*pmaster.CardImage, error) {
	cardImages, err := mcommon.FindMany[mstruct.CardImage, pmaster.CardImage](
		ctx,
		mname.CardImage,
		bson.M{"CardId": cardId},
	)
	if err != nil {
		return nil, err
	}
	return cardImages, nil
}
