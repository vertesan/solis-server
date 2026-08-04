package opsolis

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/solis/sname"
	"github.com/vertesan/solis-server/internal/db/def/solis/sstruct"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"

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
