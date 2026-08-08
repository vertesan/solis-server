package opsolis

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindQseed(ctx context.Context, deviceId string) (*smodel.Qseed, error) {
	return scommon.FindOne[smodel.Qseed](
		ctx,
		sname.Qseed,
		bson.M{"device": deviceId},
	)
}

func UpsertQseed(ctx context.Context, qseed *smodel.Qseed) error {
	return scommon.UpsertOne(
		ctx,
		sname.Qseed,
		bson.M{"device": qseed.Device},
		qseed,
	)
}

// FindOrInsertQseed atomically returns the seed already assigned to a device or
// persists and returns qseed when the device has no assignment yet.
func FindOrInsertQseed(ctx context.Context, qseed *smodel.Qseed) (*smodel.Qseed, error) {
	persisted, err := scommon.FindAndUpdateOne[smodel.Qseed](
		ctx,
		sname.Qseed,
		bson.M{"device": qseed.Device},
		bson.M{"$setOnInsert": qseed},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	)
	if mongo.IsDuplicateKeyError(err) {
		// Another request inserted this device between our match and upsert.
		// Return that winning seed instead of exposing the benign race.
		return FindQseed(ctx, qseed.Device)
	}
	return persisted, err
}
