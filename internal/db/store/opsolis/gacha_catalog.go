package opsolis

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindGachaCatalog(ctx context.Context) (*smodel.GachaCatalog, error) {
	return scommon.FindOne[smodel.GachaCatalog](ctx, sname.GachaCatalog, bson.M{})
}

func UpsertGachaCatalog(ctx context.Context, catalog *smodel.GachaCatalog) error {
	if err := catalog.Validate(); err != nil {
		return err
	}
	return scommon.UpsertOne(ctx, sname.GachaCatalog, bson.M{}, catalog)
}

func DeleteGachaCatalog(ctx context.Context) error {
	return scommon.Delete(ctx, sname.GachaCatalog, bson.M{})
}

func FindGachaRules(ctx context.Context) (*smodel.GachaRules, error) {
	return scommon.FindOne[smodel.GachaRules](ctx, sname.GachaRules, bson.M{})
}

func UpsertGachaRules(ctx context.Context, rules *smodel.GachaRules) error {
	if err := rules.Validate(); err != nil {
		return err
	}
	return scommon.UpsertOne(ctx, sname.GachaRules, bson.M{}, rules)
}

func DeleteGachaRules(ctx context.Context) error {
	return scommon.Delete(ctx, sname.GachaRules, bson.M{})
}
