package opsolis

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

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
