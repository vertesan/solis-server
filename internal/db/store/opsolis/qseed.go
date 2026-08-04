package opsolis

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/solis/sname"
	"github.com/vertesan/solis-server/internal/db/def/solis/sstruct"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"

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
