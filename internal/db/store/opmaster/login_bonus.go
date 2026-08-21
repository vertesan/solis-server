package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindAllLoginBonuses(ctx context.Context) ([]*pmaster.LoginBonus, error) {
	return mcommon.FindMany[mmodel.LoginBonus, pmaster.LoginBonus](
		ctx,
		mname.LoginBonus,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Id", Value: 1}}),
	)
}

func FindLoginBonus(ctx context.Context, id string) (*pmaster.LoginBonus, error) {
	return mcommon.FindOne[mmodel.LoginBonus, pmaster.LoginBonus](ctx, mname.LoginBonus, bson.M{"Id": id})
}

func FindAllLoginBonusTexts(ctx context.Context) ([]*pmaster.LoginBonusText, error) {
	return mcommon.FindMany[mmodel.LoginBonusText, pmaster.LoginBonusText](
		ctx,
		mname.LoginBonusText,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Id", Value: 1}}),
	)
}
