package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserCompany(ctx context.Context, suid string) (*ptransaction.UserCompany, error) {
	return ucommon.FindOne[ptransaction.UserCompany](
		ctx,
		uname.UserCompany,
		bson.M{"SUID": suid},
	)
}

func UpsertUserCompany(ctx context.Context, suid string, company *ptransaction.UserCompany) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserCompany,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*ptransaction.UserCompany]{
			SUID: suid,
			Data: company,
		},
	)
}

func DeleteUserCompany(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserCompany,
		bson.M{"SUID": suid},
	)
}
