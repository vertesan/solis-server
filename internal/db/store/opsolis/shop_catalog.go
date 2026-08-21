package opsolis

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindAllShopCatalogEntries(ctx context.Context) ([]*smodel.ShopCatalogEntry, error) {
	return scommon.FindAll[smodel.ShopCatalogEntry](
		ctx,
		sname.ShopCatalog,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "info.order", Value: 1}, {Key: "shopId", Value: 1}}),
	)
}

func FindShopCatalogEntry(ctx context.Context, shopId string) (*smodel.ShopCatalogEntry, error) {
	return scommon.FindOne[smodel.ShopCatalogEntry](ctx, sname.ShopCatalog, bson.M{"shopId": shopId})
}

func FindShopCatalogEntryByItemId(ctx context.Context, shopItemId string) (*smodel.ShopCatalogEntry, error) {
	return scommon.FindOne[smodel.ShopCatalogEntry](ctx, sname.ShopCatalog, bson.M{"items.id": shopItemId})
}

func UpsertShopCatalogEntry(ctx context.Context, entry *smodel.ShopCatalogEntry) error {
	return scommon.UpsertOne(ctx, sname.ShopCatalog, bson.M{"shopId": entry.ShopId}, entry)
}

func DeleteAllShopCatalogEntries(ctx context.Context) error {
	return scommon.Delete(ctx, sname.ShopCatalog, bson.M{})
}
