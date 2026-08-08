package opsolis

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/solis/sdoc"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindMigration(ctx context.Context) (*smodel.Migration, error) {
	return scommon.FindOne[smodel.Migration](
		ctx,
		sname.Migration,
		bson.M{"_id": sdoc.MigrationDocumentID},
	)
}

func UpsertMigration(ctx context.Context, migration *smodel.Migration) error {
	return scommon.UpsertOne(
		ctx,
		sname.Migration,
		bson.M{"_id": sdoc.MigrationDocumentID},
		migration,
	)
}
