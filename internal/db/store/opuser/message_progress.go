package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
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

func UpsertManyUserMessageProgresses(ctx context.Context, progresses []*umodel.UserMessageProgress) error {
	models := make([]mongo.WriteModel, 0, len(progresses))
	for _, progress := range progresses {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"SUID": progress.SUID, "messageId": progress.MessageId}).
			SetUpdate(bson.M{"$set": progress}).
			SetUpsert(true))
	}
	return bulkWriteGodData(ctx, uname.UserMessageProgress, models)
}

func DeleteUserMessageProgress(ctx context.Context, suid, messageID string) error {
	return ucommon.Delete(
		ctx,
		uname.UserMessageProgress,
		bson.M{"SUID": suid, "messageId": messageID},
	)
}
