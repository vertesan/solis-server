package opsolis

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindNoticeCatalog(ctx context.Context) (*smodel.NoticeCatalog, error) {
	return scommon.FindOne[smodel.NoticeCatalog](
		ctx,
		sname.NoticeCatalog,
		bson.M{},
	)
}

func UpsertNoticeCatalog(ctx context.Context, catalog *smodel.NoticeCatalog) error {
	return scommon.UpsertOne(
		ctx,
		sname.NoticeCatalog,
		bson.M{},
		catalog,
	)
}

func DeleteNoticeCatalog(ctx context.Context) error {
	return scommon.Delete(
		ctx,
		sname.NoticeCatalog,
		bson.M{},
	)
}
