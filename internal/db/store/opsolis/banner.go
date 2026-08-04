package opsolis

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindAllMenuBanners(ctx context.Context) ([]*papi.Banner, error) {
	return scommon.FindAll[papi.Banner](
		ctx,
		sname.MenuBanner,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "order", Value: 1}}),
	)
}

func UpsertMenuBanner(ctx context.Context, banner *papi.Banner) error {
	return scommon.UpsertOne(
		ctx,
		sname.MenuBanner,
		bson.M{"id": banner.Id},
		banner,
	)
}

func DeleteMenuBanner(ctx context.Context, id string) error {
	return scommon.Delete(
		ctx,
		sname.MenuBanner,
		bson.M{"id": id},
	)
}

func DeleteAllMenuBanners(ctx context.Context) error {
	return scommon.Delete(
		ctx,
		sname.MenuBanner,
		bson.M{},
	)
}
