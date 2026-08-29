package servers

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const giftPageSize = 100

type GiftServer struct {
	papi.UnimplementedGiftServer
}

func (s *GiftServer) List(ctx context.Context, req *papi.GiftListRequest) (*papi.GiftListResponse, error) {
	if req.GetOffset() < 0 {
		return nil, status.Error(codes.InvalidArgument, "offset must not be negative")
	}
	suid := viber.GetSuid(ctx).String()
	gifts, err := opuser.FindAllUserGifts(ctx, suid)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	filtered := make([]*ptransaction.UserGift, 0, len(gifts))
	for _, gift := range gifts {
		if gift.GetLimitTime() > 0 && gift.GetLimitTime() <= now || !matchesGiftFilters(gift.GetResourceType(), req.GetFilters()) {
			continue
		}
		filtered = append(filtered, gift)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left, right := filtered[i], filtered[j]
		var less bool
		switch req.GetSortType() {
		case penum.GiftSortType_GiftSortType_LimitDatetime:
			leftLimit, rightLimit := left.GetLimitTime(), right.GetLimitTime()
			if leftLimit == 0 {
				leftLimit = math.MaxInt64
			}
			if rightLimit == 0 {
				rightLimit = math.MaxInt64
			}
			less = leftLimit < rightLimit || leftLimit == rightLimit && left.GetGiftId() < right.GetGiftId()
		default:
			less = left.GetPostedTime() < right.GetPostedTime() || left.GetPostedTime() == right.GetPostedTime() && left.GetGiftId() < right.GetGiftId()
		}
		if req.GetDesc() {
			return !less && left.GetGiftId() != right.GetGiftId()
		}
		return less
	})

	offset := int(req.GetOffset())
	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := min(offset+giftPageSize, len(filtered))
	return &papi.GiftListResponse{
		Gifts:          filtered[offset:end],
		HasNext:        end < len(filtered),
		Count:          int32(len(filtered)),
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

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
			granted, err := viber.GrantGiftReward(ctx, suid, gift, updated)
			if err != nil {
				if errors.Is(err, viber.ErrPhotoGiftCapacity) || errors.Is(err, viber.ErrPhotoRecipeGiftCapacity) {
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

func (s *GiftServer) ListHistory(ctx context.Context, _ *papi.Empty) (*papi.GiftHistoryListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	history, err := opuser.FindAllUserGiftHistories(ctx, suid)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(history, func(i, j int) bool {
		if history[i].GetReceivedTime() == history[j].GetReceivedTime() {
			return history[i].GetGiftId() < history[j].GetGiftId()
		}
		return history[i].GetReceivedTime() > history[j].GetReceivedTime()
	})
	if len(history) > giftPageSize {
		history = history[:giftPageSize]
	}
	return &papi.GiftHistoryListResponse{Gifts: history, CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func matchesGiftFilters(resourceType penum.ResourceType, filters []penum.GiftFilterType) bool {
	if len(filters) == 0 {
		return true
	}
	category := penum.GiftFilterType_GiftFilterType_Other
	switch resourceType {
	case penum.ResourceType_ResourceType_Stone, penum.ResourceType_ResourceType_PaidStone:
		category = penum.GiftFilterType_GiftFilterType_Stone
	case penum.ResourceType_ResourceType_Card:
		category = penum.GiftFilterType_GiftFilterType_Card
	case penum.ResourceType_ResourceType_PhotoSituation,
		penum.ResourceType_ResourceType_PhotoAllInOne,
		penum.ResourceType_ResourceType_PhotoGift,
		penum.ResourceType_ResourceType_PhotoRecipe:
		category = penum.GiftFilterType_GiftFilterType_Photo
	case penum.ResourceType_ResourceType_Accessory:
		category = penum.GiftFilterType_GiftFilterType_Accessory
	case penum.ResourceType_ResourceType_Gold:
		category = penum.GiftFilterType_GiftFilterType_Gold
	}
	for _, filter := range filters {
		if filter == category {
			return true
		}
	}
	return false
}
