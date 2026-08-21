package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserLoginBonuses(ctx context.Context, suid string) ([]*ptransaction.UserLoginBonus, error) {
	return ucommon.FindAll[ptransaction.UserLoginBonus](ctx, uname.UserLoginBonus, bson.M{"SUID": suid})
}

func FindUserLoginBonus(ctx context.Context, suid, loginBonusID string) (*ptransaction.UserLoginBonus, error) {
	return ucommon.FindOne[ptransaction.UserLoginBonus](
		ctx,
		uname.UserLoginBonus,
		bson.M{"SUID": suid, "loginBonusId": loginBonusID},
	)
}

func UpsertUserLoginBonus(ctx context.Context, suid string, bonus *ptransaction.UserLoginBonus) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserLoginBonus,
		bson.M{"SUID": suid, "loginBonusId": bonus.GetLoginBonusId()},
		&umodel.UserStruct[*ptransaction.UserLoginBonus]{SUID: suid, Data: bonus},
	)
}
