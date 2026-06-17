package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserCurrentQuestArea(ctx context.Context, suid string) (*ustruct.UserCurrentQuestArea, error) {
	return ucommon.FindOne[ustruct.UserCurrentQuestArea](
		ctx,
		uname.UserCurrentQuestArea,
		bson.M{"SUID": suid},
	)
}

func UpsertUserCurrentQuestArea(ctx context.Context, data *ustruct.UserCurrentQuestArea) error {
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
