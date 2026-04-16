package opsolis

import (
	"context"
	"vertesan/solis-server/db/crud/opsolis/internal/scommon"
	"vertesan/solis-server/db/def/solis/sname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func UpsertMasterTagPack(ctx context.Context, masterTagPack *ptransaction.MasterTagPack) error {
	return scommon.UpsertOne(
		ctx,
		sname.MasterTagPack,
		bson.M{"type": masterTagPack.Type},
		masterTagPack,
	)
}

func GetAllMasterTagPack(ctx context.Context) ([](*ptransaction.MasterTagPack), error) {
	return scommon.FindAll[ptransaction.MasterTagPack](
		ctx,
		sname.MasterTagPack,
		bson.M{},
	)
}
