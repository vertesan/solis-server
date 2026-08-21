package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserMessageProgress(ctx context.Context, suid string, messageId string) (*umodel.UserMessageProgress, error) {
	return ucommon.FindOne[umodel.UserMessageProgress](
		ctx,
		uname.UserMessageProgress,
		bson.M{"SUID": suid, "messageId": messageId},
	)
}

func FindAllUserMessageProgresses(ctx context.Context, suid string) ([]*umodel.UserMessageProgress, error) {
	return ucommon.FindAll[umodel.UserMessageProgress](
		ctx,
		uname.UserMessageProgress,
		bson.M{"SUID": suid},
	)
}

func UpsertUserMessageProgress(ctx context.Context, progress *umodel.UserMessageProgress) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserMessageProgress,
		bson.M{"SUID": progress.SUID, "messageId": progress.MessageId},
		progress,
	)
}
