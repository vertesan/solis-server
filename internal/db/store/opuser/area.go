package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func UpsertUserArea(ctx context.Context, suid string, area *ptransaction.UserArea) error {
	return ucommon.UpsertOne(ctx, uname.UserArea, bson.M{"SUID": suid, "areaId": area.GetAreaId()},
		&umodel.UserStruct[*ptransaction.UserArea]{SUID: suid, Data: area})
}

func FindAllUserAreas(ctx context.Context, suid string) ([]*ptransaction.UserArea, error) {
	return ucommon.FindAll[ptransaction.UserArea](
		ctx,
		uname.UserArea,
		bson.M{"SUID": suid},
	)
}
