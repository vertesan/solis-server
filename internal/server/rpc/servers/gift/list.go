package gift

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
