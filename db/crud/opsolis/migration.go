package opsolis

import (
	"context"
	"vertesan/solis-server/db/crud/opsolis/internal/scommon"
	"vertesan/solis-server/db/def/solis/sname"
	"vertesan/solis-server/db/def/solis/sstruct"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindMigration(ctx context.Context) (*sstruct.Migration, error) {
	return scommon.FindOne[sstruct.Migration](
		ctx,
		sname.Migration,
		bson.M{},
	)
}

func UpsertMigration(ctx context.Context, migration *sstruct.Migration) error {
	return scommon.UpsertOne(
		ctx,
		sname.Migration,
		bson.M{},
		migration,
	)
}
