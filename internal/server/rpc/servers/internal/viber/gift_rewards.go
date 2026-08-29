package viber

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrPhotoGiftCapacity lets Gift.Receive report a capacity-blocked photo as
// unreceived without failing the whole mailbox request.
var ErrPhotoGiftCapacity = errors.New("photo gift exceeds owned-photo capacity")

// ErrPhotoRecipeGiftCapacity lets Gift.Receive leave a recipe gift in the
// mailbox until the user has sold enough recipes to receive it in full.
var ErrPhotoRecipeGiftCapacity = errors.New("photo recipe gift exceeds recipe capacity")

// GrantGiftReward applies the mailbox-specific state carried by a UserGift,
// then delegates ordinary resources to the canonical reward grant path.
func GrantGiftReward(
	ctx context.Context,
	suid string,
	gift *ptransaction.UserGift,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if gift == nil {
		return nil, status.Error(codes.FailedPrecondition, "gift is required")
	}
	if gift.GetAmount() <= 0 || gift.GetAmount() > math.MaxInt32 {
		return nil, status.Error(codes.FailedPrecondition, "gift amount is invalid")
	}
	if gift.GetLimitTime() < 0 {
		return nil, status.Error(codes.FailedPrecondition, "gift limit time is invalid")
	}
	switch gift.GetResourceType() {
	case penum.ResourceType_ResourceType_PhotoGift:
		return grantPhotoGift(ctx, suid, gift, updated)
	case penum.ResourceType_ResourceType_PhotoAllInOne:
		owned, limit, err := FindPhotoCapacity(ctx, suid)
		if err != nil {
			return nil, err
		}
		if owned >= limit || gift.GetAmount() > limit-owned {
			return nil, ErrPhotoGiftCapacity
		}
	case penum.ResourceType_ResourceType_PhotoRecipe:
		owned, limit, err := FindPhotoRecipeCapacity(ctx, suid)
		if err != nil {
			return nil, err
		}
		if owned >= limit || gift.GetAmount() > limit-owned {
			return nil, ErrPhotoRecipeGiftCapacity
		}
	}
	if gift.GetResourceType() == penum.ResourceType_ResourceType_Item && gift.GetLimitTime() > 0 {
		if gift.GetLimitTime() <= time.Now().UnixMilli() {
			return nil, status.Error(codes.FailedPrecondition, "gift has expired")
		}
		result, err := grantExchangeItemWithExpiration(
			ctx,
			suid,
			gift.GetResourceId(),
			gift.GetAmount(),
			gift.GetLimitTime(),
			updated,
		)
		if err != nil {
			return nil, err
		}
		return []*ptransaction.RewardResult{result}, nil
	}
	return GrantGachaReward(ctx, suid, &pmaster.RewardResource{
		ResourceType: gift.GetResourceType(),
		ResourceId:   gift.GetResourceId(),
		Amount:       int32(gift.GetAmount()),
	}, updated)
}

func grantPhotoGift(
	ctx context.Context,
	suid string,
	gift *ptransaction.UserGift,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if gift.GetAmount() != 1 || gift.GetResourceId() == "" || gift.GetPhotoGift() == nil {
		return nil, status.Error(codes.FailedPrecondition, "photo gift state is invalid")
	}
	photo, err := opuser.FindUserPhotoGift(ctx, suid, gift.GetResourceId())
	if err != nil {
		return nil, err
	}
	if photo.GetPhotoId() != gift.GetResourceId() {
		return nil, status.Error(codes.FailedPrecondition, "photo gift identity is inconsistent")
	}
	if _, err := opuser.FindUserPhoto(ctx, suid, photo.GetPhotoId()); err == nil {
		return nil, status.Error(codes.FailedPrecondition, "photo gift is already owned")
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}

	before, capacity, err := FindPhotoCapacity(ctx, suid)
	if err != nil {
		return nil, err
	}
	if before >= capacity {
		return nil, ErrPhotoGiftCapacity
	}

	if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
		return nil, err
	}
	if err := opuser.DeleteUserPhotoGift(ctx, suid, photo.GetPhotoId()); err != nil {
		return nil, err
	}
	updated.Photos = append(updated.Photos, photo)
	return []*ptransaction.RewardResult{{
		ResourceType: penum.ResourceType_ResourceType_PhotoGift,
		ResourceId:   photo.GetPhotoId(),
		BeforeAmount: before,
		AfterAmount:  before + 1,
		IsNew:        true,
		AmountI64:    1,
	}}, nil
}

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
