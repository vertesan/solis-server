package opsolis

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindHomeEventInfo(ctx context.Context) (*papi.HomeEventInfo, error) {
	return scommon.FindOne[papi.HomeEventInfo](
		ctx,
		sname.HomeEventInfo,
		bson.M{},
	)
}

func UpsertHomeEventInfo(ctx context.Context, info *papi.HomeEventInfo) error {
	return scommon.UpsertOne(
		ctx,
		sname.HomeEventInfo,
		bson.M{},
		info,
	)
}

func DeleteHomeEventInfo(ctx context.Context) error {
	return scommon.Delete(
		ctx,
		sname.HomeEventInfo,
		bson.M{},
	)
}
