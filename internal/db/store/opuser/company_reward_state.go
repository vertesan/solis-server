package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserCompanyRewardState(ctx context.Context, suid string) (*umodel.UserCompanyRewardState, error) {
	return ucommon.FindOne[umodel.UserCompanyRewardState](
		ctx,
		uname.UserCompanyRewardState,
		bson.M{"SUID": suid},
	)
}

func UpsertUserCompanyRewardState(ctx context.Context, state *umodel.UserCompanyRewardState) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserCompanyRewardState,
		bson.M{"SUID": state.SUID},
		state,
	)
}

func DeleteUserCompanyRewardState(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserCompanyRewardState, bson.M{"SUID": suid})
}
