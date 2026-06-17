package opsolis

import (
	"context"
	"vertesan/solis-server/db/def/solis/sname"
	"vertesan/solis-server/db/def/solis/sstruct"
	"vertesan/solis-server/db/store/opsolis/internal/scommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindQseed(ctx context.Context, deviceId string) (*sstruct.Qseed, error) {
	return scommon.FindOne[sstruct.Qseed](
		ctx,
		sname.Qseed,
		bson.M{"device": deviceId},
	)
}

func UpsertQseed(ctx context.Context, qseed *sstruct.Qseed) error {
	return scommon.UpsertOne(
		ctx,
		sname.Qseed,
		bson.M{"device": qseed.Device},
		qseed,
	)
}
