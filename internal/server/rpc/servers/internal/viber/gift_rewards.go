package viber

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// SetGiftNotification persists and returns a mailbox notification only when
// its visible validity actually changes.
func SetGiftNotification(ctx context.Context, suid string, valid bool, updated *papi.UpdatedData) error {
	for _, notification := range updated.GetNotifications() {
		if notification.GetNotificationType() != penum.NotificationType_NotificationType_Gift {
			continue
		}
		if notification.GetValid() == valid {
			return nil
		}
		notification.Valid = valid
		return opuser.UpsertUserNotification(ctx, suid, notification)
	}

	notification, err := opuser.FindUserNotification(
		ctx, suid, penum.NotificationType_NotificationType_Gift,
	)
	if err == nil && notification.GetValid() == valid {
		return nil
	}
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	if notification == nil {
		notification = &ptransaction.UserNotification{
			NotificationType: penum.NotificationType_NotificationType_Gift,
		}
	}
	notification.Valid = valid
	if err := opuser.UpsertUserNotification(ctx, suid, notification); err != nil {
		return err
	}
	updated.Notifications = append(updated.Notifications, notification)
	return nil
}
