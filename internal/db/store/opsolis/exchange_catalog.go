package opsolis

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindAllExchangeBooths(ctx context.Context) ([]*papi.ExchangeBooth, error) {
	return scommon.FindAll[papi.ExchangeBooth](
		ctx,
		sname.ExchangeCatalog,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "order", Value: 1}, {Key: "id", Value: 1}}),
	)
}

func FindExchangeBoothByExchangeId(ctx context.Context, exchangeId string) (*papi.ExchangeBooth, error) {
	return scommon.FindOne[papi.ExchangeBooth](
		ctx,
		sname.ExchangeCatalog,
		bson.M{"exchanges.id": exchangeId},
	)
}

func UpsertExchangeBooth(ctx context.Context, booth *papi.ExchangeBooth) error {
	return scommon.UpsertOne(ctx, sname.ExchangeCatalog, bson.M{"id": booth.Id}, booth)
}

func DeleteAllExchangeBooths(ctx context.Context) error {
	return scommon.Delete(ctx, sname.ExchangeCatalog, bson.M{})
}
