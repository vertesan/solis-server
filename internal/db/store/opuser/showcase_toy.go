package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserShowcaseToy(ctx context.Context, suid, showcaseToyId string) (*ptransaction.UserShowcaseToy, error) {
	return ucommon.FindOne[ptransaction.UserShowcaseToy](
		ctx, uname.UserShowcaseToy, bson.M{"SUID": suid, "showcaseToyId": showcaseToyId},
	)
}

func FindAllUserShowcaseToys(ctx context.Context, suid string) ([]*ptransaction.UserShowcaseToy, error) {
	return ucommon.FindAll[ptransaction.UserShowcaseToy](
		ctx,
		uname.UserShowcaseToy,
		bson.M{"SUID": suid},
	)
}

func UpsertUserShowcaseToy(ctx context.Context, suid string, toy *ptransaction.UserShowcaseToy) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShowcaseToy,
		bson.M{"SUID": suid, "showcaseToyId": toy.ShowcaseToyId},
		&umodel.UserStruct[*ptransaction.UserShowcaseToy]{SUID: suid, Data: toy},
	)
}
