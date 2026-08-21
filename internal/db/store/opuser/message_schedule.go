package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserMessageSchedule(ctx context.Context, suid string, messageId string) (*ptransaction.UserMessageSchedule, error) {
	return ucommon.FindOne[ptransaction.UserMessageSchedule](
		ctx,
		uname.UserMessageSchedule,
		bson.M{"SUID": suid, "messageId": messageId},
	)
}

func FindAllUserMessageSchedules(ctx context.Context, suid string) ([]*ptransaction.UserMessageSchedule, error) {
	return ucommon.FindAll[ptransaction.UserMessageSchedule](
		ctx,
		uname.UserMessageSchedule,
		bson.M{"SUID": suid},
	)
}

func UpsertUserMessageSchedule(ctx context.Context, suid string, schedule *ptransaction.UserMessageSchedule) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserMessageSchedule,
		bson.M{"SUID": suid, "messageId": schedule.MessageId},
		&umodel.UserStruct[*ptransaction.UserMessageSchedule]{SUID: suid, Data: schedule},
	)
}

func DeleteUserMessageSchedule(ctx context.Context, suid string, messageId string) error {
	return ucommon.Delete(
		ctx,
		uname.UserMessageSchedule,
		bson.M{"SUID": suid, "messageId": messageId},
	)
}
