package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserCurrentQuestArea(ctx context.Context, suid string) (*umodel.UserCurrentQuestArea, error) {
	return ucommon.FindOne[umodel.UserCurrentQuestArea](
		ctx,
		uname.UserCurrentQuestArea,
		bson.M{"SUID": suid},
	)
}

func UpsertUserCurrentQuestArea(ctx context.Context, data *umodel.UserCurrentQuestArea) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserCurrentQuestArea,
		bson.M{"SUID": data.SUID},
		data,
	)
}

func DeleteUserCurrentQuestArea(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserCurrentQuestArea,
		bson.M{"SUID": suid},
	)
}
