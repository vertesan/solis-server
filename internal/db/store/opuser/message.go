package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserMessage(ctx context.Context, suid string, messageId string) (*ptransaction.UserMessage, error) {
	return ucommon.FindOne[ptransaction.UserMessage](
		ctx,
		uname.UserMessage,
		bson.M{"SUID": suid, "messageId": messageId},
	)
}

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
		&umodel.UserStruct[*ptransaction.UserMessage]{
			SUID: suid,
			Data: message,
		},
	)
}

func UpsertManyUserMessages(ctx context.Context, suid string, messages []*ptransaction.UserMessage) error {
	return ucommon.ForEachAtomically(ctx, messages, func(ctx context.Context, message *ptransaction.UserMessage) error {
		return UpsertUserMessage(ctx, suid, message)
	})
}

func DeleteAllUserMessages(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserMessage,
		bson.M{"SUID": suid},
	)
}
