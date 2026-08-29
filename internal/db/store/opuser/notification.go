package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserNotification(
	ctx context.Context,
	suid string,
	notificationType penum.NotificationType,
) (*ptransaction.UserNotification, error) {
	return ucommon.FindOne[ptransaction.UserNotification](
		ctx,
		uname.UserNotification,
		bson.M{"SUID": suid, "notificationType": notificationType},
	)
}

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
		&umodel.UserStruct[*ptransaction.UserNotification]{
			SUID: suid,
			Data: notification,
		},
	)
}

func UpsertManyUserNotifications(ctx context.Context, suid string, notifications []*ptransaction.UserNotification) error {
	return ucommon.ForEachAtomically(ctx, notifications, func(ctx context.Context, notification *ptransaction.UserNotification) error {
		return UpsertUserNotification(ctx, suid, notification)
	})
}

func DeleteAllUserNotifications(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserNotification,
		bson.M{"SUID": suid},
	)
}
