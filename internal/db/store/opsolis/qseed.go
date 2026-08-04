package opsolis

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"

	"go.mongodb.org/mongo-driver/v2/bson"
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
