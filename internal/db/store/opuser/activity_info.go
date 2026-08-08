package opuser

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ---------- FanEventInfo ----------

func FindUserFanEventInfo(ctx context.Context, suid string) (*papi.FanEventInfo, error) {
	return ucommon.FindOne[papi.FanEventInfo](
		ctx,
		uname.UserFanEventInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserFanEventInfo(ctx context.Context, suid string, info *papi.FanEventInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserFanEventInfo,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*papi.FanEventInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func DeleteUserFanEventInfo(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserFanEventInfo,
		bson.M{"SUID": suid},
	)
}

func FindUserFanEvent(ctx context.Context, suid string, fanEventId string) (*papi.FanEvent, error) {
	info, err := FindUserFanEventInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	for _, e := range info.FanEvents {
		if e.Id == fanEventId {
			return e, nil
		}
	}
	return nil, fmt.Errorf("fanEvent %q not found for suid %q", fanEventId, suid)
}

func UpsertUserFanEvent(ctx context.Context, suid string, fanEvent *papi.FanEvent) error {
	return mutateUserFanEventInfo(ctx, suid, func(info *papi.FanEventInfo) error {
		replaced := false
		for i, e := range info.FanEvents {
			if e.Id == fanEvent.Id {
				info.FanEvents[i] = fanEvent
				replaced = true
				break
			}
		}
		if !replaced {
			info.FanEvents = append(info.FanEvents, fanEvent)
		}
		return nil
	})
}

func FindUserActiveFanEvent(ctx context.Context, suid string) (*papi.ActiveFanEvent, error) {
	info, err := FindUserFanEventInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	return info.ActiveFanEvent, nil
}

func InsertUserActiveFanEvent(ctx context.Context, suid string, active *papi.ActiveFanEvent) error {
	return mutateUserFanEventInfo(ctx, suid, func(info *papi.FanEventInfo) error {
		if info.ActiveFanEvent != nil {
			return fmt.Errorf("an active fanEvent already exists for suid %q", suid)
		}
		info.ActiveFanEvent = active
		return nil
	})
}

func DeleteUserActiveFanEvent(ctx context.Context, suid string) error {
	return mutateUserFanEventInfo(ctx, suid, func(info *papi.FanEventInfo) error {
		info.ActiveFanEvent = nil
		return nil
	})
}

// mutateUserFanEventInfo performs a fan-event read, in-memory mutation, and
// replacement in one transaction to prevent lost updates.
func mutateUserFanEventInfo(ctx context.Context, suid string, mutate func(*papi.FanEventInfo) error) error {
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		info, err := FindUserFanEventInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		if err := mutate(info); err != nil {
			return nil, err
		}
		return nil, UpsertUserFanEventInfo(ctx, suid, info)
	})
}

// ---------- PromotionInfo ----------

func FindUserPromotionInfo(ctx context.Context, suid string) (*papi.PromotionInfo, error) {
	return ucommon.FindOne[papi.PromotionInfo](
		ctx,
		uname.UserPromotionInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPromotionInfo(ctx context.Context, suid string, info *papi.PromotionInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPromotionInfo,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*papi.PromotionInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func DeleteUserPromotionInfo(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPromotionInfo,
		bson.M{"SUID": suid},
	)
}

func FindUserPromotion(ctx context.Context, suid string, promotionId string) (*papi.Promotion, error) {
	info, err := FindUserPromotionInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	for _, p := range info.Promotions {
		if p.Id == promotionId {
			return p, nil
		}
	}
	return nil, fmt.Errorf("promotion %q not found for suid %q", promotionId, suid)
}

func UpsertUserPromotion(ctx context.Context, suid string, promotion *papi.Promotion) error {
	return mutateUserPromotionInfo(ctx, suid, func(info *papi.PromotionInfo) error {
		replaced := false
		for i, p := range info.Promotions {
			if p.Id == promotion.Id {
				info.Promotions[i] = promotion
				replaced = true
				break
			}
		}
		if !replaced {
			info.Promotions = append(info.Promotions, promotion)
		}
		return nil
	})
}

func FindUserActivePromotion(ctx context.Context, suid string) (*papi.ActivePromotion, error) {
	info, err := FindUserPromotionInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	return info.ActivePromotion, nil
}

func InsertUserActivePromotion(ctx context.Context, suid string, active *papi.ActivePromotion) error {
	return mutateUserPromotionInfo(ctx, suid, func(info *papi.PromotionInfo) error {
		if info.ActivePromotion != nil {
			return fmt.Errorf("an active promotion already exists for suid %q", suid)
		}
		info.ActivePromotion = active
		return nil
	})
}

func DeleteUserActivePromotion(ctx context.Context, suid string) error {
	return mutateUserPromotionInfo(ctx, suid, func(info *papi.PromotionInfo) error {
		info.ActivePromotion = nil
		return nil
	})
}

// mutateUserPromotionInfo performs a promotion read, in-memory mutation, and
// replacement in one transaction to prevent lost updates.
func mutateUserPromotionInfo(ctx context.Context, suid string, mutate func(*papi.PromotionInfo) error) error {
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		info, err := FindUserPromotionInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		if err := mutate(info); err != nil {
			return nil, err
		}
		return nil, UpsertUserPromotionInfo(ctx, suid, info)
	})
}

// ---------- RefreshInfo ----------

func FindUserRefreshInfo(ctx context.Context, suid string) (*papi.RefreshInfo, error) {
	return ucommon.FindOne[papi.RefreshInfo](
		ctx,
		uname.UserRefreshInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserRefreshInfo(ctx context.Context, suid string, info *papi.RefreshInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserRefreshInfo,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*papi.RefreshInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func DeleteUserRefreshInfo(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserRefreshInfo,
		bson.M{"SUID": suid},
	)
}

func FindUserRefresh(ctx context.Context, suid string, refreshId string) (*papi.Refresh, error) {
	info, err := FindUserRefreshInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	for _, r := range info.Refreshes {
		if r.Id == refreshId {
			return r, nil
		}
	}
	return nil, fmt.Errorf("refresh %q not found for suid %q", refreshId, suid)
}

func UpsertUserRefresh(ctx context.Context, suid string, refresh *papi.Refresh) error {
	return mutateUserRefreshInfo(ctx, suid, func(info *papi.RefreshInfo) error {
		replaced := false
		for i, r := range info.Refreshes {
			if r.Id == refresh.Id {
				info.Refreshes[i] = refresh
				replaced = true
				break
			}
		}
		if !replaced {
			info.Refreshes = append(info.Refreshes, refresh)
		}
		return nil
	})
}

func FindUserActiveRefresh(ctx context.Context, suid string) (*papi.ActiveRefresh, error) {
	info, err := FindUserRefreshInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	return info.ActiveRefresh, nil
}

func InsertUserActiveRefresh(ctx context.Context, suid string, active *papi.ActiveRefresh) error {
	return mutateUserRefreshInfo(ctx, suid, func(info *papi.RefreshInfo) error {
		if info.ActiveRefresh != nil {
			return fmt.Errorf("an active refresh already exists for suid %q", suid)
		}
		info.ActiveRefresh = active
		return nil
	})
}

func DeleteUserActiveRefresh(ctx context.Context, suid string) error {
	return mutateUserRefreshInfo(ctx, suid, func(info *papi.RefreshInfo) error {
		info.ActiveRefresh = nil
		return nil
	})
}

// mutateUserRefreshInfo performs a refresh read, in-memory mutation, and
// replacement in one transaction to prevent lost updates.
func mutateUserRefreshInfo(ctx context.Context, suid string, mutate func(*papi.RefreshInfo) error) error {
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		info, err := FindUserRefreshInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		if err := mutate(info); err != nil {
			return nil, err
		}
		return nil, UpsertUserRefreshInfo(ctx, suid, info)
	})
}
