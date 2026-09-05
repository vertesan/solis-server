package gift

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *GiftServer) Receive(ctx context.Context, req *papi.GiftReceiveRequest) (*papi.GiftReceiveResponse, error) {
	if len(req.GetGiftIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "giftIds is required")
	}
	seen := make(map[string]struct{}, len(req.GetGiftIds()))
	for _, giftID := range req.GetGiftIds() {
		if giftID == "" {
			return nil, status.Error(codes.InvalidArgument, "giftId must not be empty")
		}
		if _, exists := seen[giftID]; exists {
			return nil, status.Error(codes.InvalidArgument, "giftIds must not contain duplicates")
		}
		seen[giftID] = struct{}{}
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.GiftReceiveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		updated := &papi.UpdatedData{}
		results := make([]*ptransaction.RewardResult, 0, len(req.GetGiftIds()))
		receivedIDs := make([]string, 0, len(req.GetGiftIds()))
		unreceivedIDs := make([]string, 0)
		for _, giftID := range req.GetGiftIds() {
			gift, err := opuser.FindUserGift(ctx, suid, giftID)
			if err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					unreceivedIDs = append(unreceivedIDs, giftID)
					continue
				}
				return nil, err
			}
			if gift.GetLimitTime() > 0 && gift.GetLimitTime() <= now {
				unreceivedIDs = append(unreceivedIDs, giftID)
				continue
			}
			if gift.GetAmount() <= 0 || gift.GetAmount() > math.MaxInt32 {
				return nil, status.Errorf(codes.FailedPrecondition, "gift %q has an invalid amount", giftID)
			}
			granted, err := grantGiftReward(ctx, suid, gift, updated)
			if err != nil {
				if errors.Is(err, ErrPhotoGiftCapacity) || errors.Is(err, ErrPhotoRecipeGiftCapacity) {
					unreceivedIDs = append(unreceivedIDs, giftID)
					continue
				}
				return nil, err
			}
			results = append(results, granted...)
			history := &ptransaction.UserGiftHistory{
				GiftId:       gift.GetGiftId(),
				ResourceType: gift.GetResourceType(),
				ResourceId:   gift.GetResourceId(),
				Amount:       gift.GetAmount(),
				Message:      gift.GetMessage(),
				PostedTime:   gift.GetPostedTime(),
				ReceivedTime: now,
			}
			if gift.GetPhotoGift() != nil {
				history.PhotoGift = &ptransaction.PhotoGiftHistory{Name: gift.GetPhotoGift().GetName()}
			}
			if err := opuser.UpsertUserGiftHistory(ctx, suid, history); err != nil {
				return nil, err
			}
			if err := opuser.DeleteUserGift(ctx, suid, giftID); err != nil {
				return nil, err
			}
			receivedIDs = append(receivedIDs, giftID)
		}
		results, err := viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}

		remaining, err := opuser.FindAllUserGifts(ctx, suid)
		if err != nil {
			return nil, err
		}
		hasUnreceived := false
		for _, gift := range remaining {
			if gift.GetLimitTime() == 0 || gift.GetLimitTime() > now {
				hasUnreceived = true
				break
			}
		}
		if len(receivedIDs) > 0 {
			if err := viber.SetGiftNotification(ctx, suid, hasUnreceived, updated); err != nil {
				return nil, err
			}
		}
		response = &papi.GiftReceiveResponse{
			Results:           results,
			HasUnreceivedGift: hasUnreceived,
			ReceivedGiftIds:   receivedIDs,
			UnreceivedGiftIds: unreceivedIDs,
			CommonResponse:    viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// grantGachaReward applies one master-data reward. Card grants also apply the
// card's first-obtain unlocks or its master-defined duplicate compensation.
func grantGachaReward(
	ctx context.Context,
	suid string,
	reward *pmaster.RewardResource,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	return viber.GrantGachaReward(ctx, suid, reward, nil, updated)
}

// ErrPhotoGiftCapacity lets Gift.Receive report a capacity-blocked photo as
// unreceived without failing the whole mailbox request.
var ErrPhotoGiftCapacity = errors.New("photo gift exceeds owned-photo capacity")

// ErrPhotoRecipeGiftCapacity lets Gift.Receive leave a recipe gift in the
// mailbox until the user has sold enough recipes to receive it in full.
var ErrPhotoRecipeGiftCapacity = errors.New("photo recipe gift exceeds recipe capacity")

// grantGiftReward applies the mailbox-specific state carried by a UserGift,
// then delegates ordinary resources to the canonical reward grant path.
func grantGiftReward(
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
		owned, limit, err := viber.FindPhotoCapacity(ctx, suid)
		if err != nil {
			return nil, err
		}
		if owned >= limit || gift.GetAmount() > limit-owned {
			return nil, ErrPhotoGiftCapacity
		}
	case penum.ResourceType_ResourceType_PhotoRecipe:
		owned, limit, err := viber.FindPhotoRecipeCapacity(ctx, suid)
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
		result, err := viber.GrantExchangeItemWithExpiration(
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
	return grantGachaReward(ctx, suid, &pmaster.RewardResource{
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

	before, capacity, err := viber.FindPhotoCapacity(ctx, suid)
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
