package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserNotifications(ctx context.Context, suid string) ([]*ptransaction.UserNotification, error) {
	return ucommon.FindAll[ptransaction.UserNotification](
		ctx,
		uname.UserNotification,
		bson.M{"SUID": suid},
	)
}

func UpsertUserNotification(ctx context.Context, suid string, notification *ptransaction.UserNotification) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserNotification,
		bson.M{"SUID": suid, "notificationType": notification.NotificationType},
		&ustruct.UserStruct[*ptransaction.UserNotification]{
			SUID: suid,
			Data: notification,
		},
	)
}

func UpsertManyUserNotifications(ctx context.Context, suid string, notifications []*ptransaction.UserNotification) error {
	for _, notification := range notifications {
		if err := UpsertUserNotification(ctx, suid, notification); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserNotifications(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserNotification,
		bson.M{"SUID": suid},
	)
}
