package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserMessages(ctx context.Context, suid string) ([]*ptransaction.UserMessage, error) {
	return ucommon.FindAll[ptransaction.UserMessage](
		ctx,
		uname.UserMessage,
		bson.M{"SUID": suid},
	)
}

func UpsertUserMessage(ctx context.Context, suid string, message *ptransaction.UserMessage) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserMessage,
		bson.M{"SUID": suid, "messageId": message.MessageId},
		&ustruct.UserStruct[*ptransaction.UserMessage]{
			SUID: suid,
			Data: message,
		},
	)
}

func UpsertManyUserMessages(ctx context.Context, suid string, messages []*ptransaction.UserMessage) error {
	for _, message := range messages {
		if err := UpsertUserMessage(ctx, suid, message); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserMessages(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserMessage,
		bson.M{"SUID": suid},
	)
}
