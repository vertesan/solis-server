package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserTelephone(ctx context.Context, suid string, telephoneId string) (*ptransaction.UserTelephone, error) {
	return ucommon.FindOne[ptransaction.UserTelephone](
		ctx,
		uname.UserTelephone,
		bson.M{"SUID": suid, "telephoneId": telephoneId},
	)
}

func FindAllUserTelephones(ctx context.Context, suid string) ([]*ptransaction.UserTelephone, error) {
	return ucommon.FindAll[ptransaction.UserTelephone](
		ctx,
		uname.UserTelephone,
		bson.M{"SUID": suid},
	)
}

func UpsertUserTelephone(ctx context.Context, suid string, telephone *ptransaction.UserTelephone) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserTelephone,
		bson.M{"SUID": suid, "telephoneId": telephone.TelephoneId},
		&umodel.UserStruct[*ptransaction.UserTelephone]{SUID: suid, Data: telephone},
	)
}

func UpsertManyUserTelephones(ctx context.Context, suid string, telephones []*ptransaction.UserTelephone) error {
	return ucommon.ForEachAtomically(ctx, telephones, func(ctx context.Context, telephone *ptransaction.UserTelephone) error {
		return UpsertUserTelephone(ctx, suid, telephone)
	})
}
