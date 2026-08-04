package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindCardImages(ctx context.Context, cardId string) ([]*pmaster.CardImage, error) {
	cardImages, err := mcommon.FindMany[mmodel.CardImage, pmaster.CardImage](
		ctx,
		mname.CardImage,
		bson.M{"CardId": cardId},
	)
	if err != nil {
		return nil, err
	}
	return cardImages, nil
}
