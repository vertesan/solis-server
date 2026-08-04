package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

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
		&umodel.UserStruct[*ptransaction.UserCompany]{
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
