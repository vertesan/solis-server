package shop

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func findShowcaseShop(c *viber.ShowcaseCommerceCatalog, shopID string) (*smodel.ShopCatalogEntry, bool) {
	if c == nil {
		return nil, false
	}
	shop, ok := c.ShopsByID[shopID]
	return shop, ok
}

// nextJSTDayStart returns the next 00:00 JST boundary.
func nextJSTDayStart(now time.Time) time.Time {
	local := now.In(viber.JstLocation)
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, viber.JstLocation)
}

// nextJSTWeekStart returns the next Sunday 00:00 JST boundary.
func nextJSTWeekStart(now time.Time) time.Time {
	local := now.In(viber.JstLocation)
	days := (7 - int(local.Weekday())) % 7
	if days == 0 {
		days = 7
	}
	return time.Date(local.Year(), local.Month(), local.Day()+days, 0, 0, 0, 0, viber.JstLocation)
}

// nextJSTMonthStart returns the first 00:00 JST boundary of the next month.
func nextJSTMonthStart(now time.Time) time.Time {
	local := now.In(viber.JstLocation)
	return time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, viber.JstLocation)
}

func findAuthorizedShopEntry(
	ctx context.Context,
	suid string,
	shopID string,
	evaluator *viber.CommerceConditionEvaluator,
) (*smodel.ShopCatalogEntry, error) {
	if viber.IsShowcaseCommerceShopID(shopID) {
		catalog, err := viber.BuildShowcaseCommerceCatalog(ctx, suid)
		if err != nil {
			return nil, err
		}
		entry, ok := findShowcaseShop(catalog, shopID)
		if !ok {
			return nil, status.Error(codes.NotFound, "shop has no available items")
		}
		visible, err := evaluator.Satisfied(entry.Info.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		if !visible {
			return nil, status.Error(codes.PermissionDenied, "shop is locked")
		}
		return entry, nil
	}
	_, err := findUserShopAccess(ctx, suid)
	if err != nil {
		return nil, err
	}
	entry, err := opsolis.FindShopCatalogEntry(ctx, shopID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.NotFound, "shop not found")
		}
		return nil, err
	}
	if entry.Info == nil {
		return nil, status.Error(codes.NotFound, "shop not found")
	}
	visible, err := evaluator.Satisfied(entry.Info.GetViewConditionId())
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, status.Error(codes.PermissionDenied, "shop is locked")
	}
	return entry, nil
}

func findUserShopAccess(ctx context.Context, suid string) (map[string]struct{}, error) {
	allowed := make(map[string]struct{})
	userShopIDs, err := opuser.FindUserShopIds(ctx, suid)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if err == nil {
		for _, shopID := range userShopIDs.ShopIds {
			allowed[shopID] = struct{}{}
		}
	}
	showcaseShopIDs, err := opuser.FindUserShowcaseToyShopIds(ctx, suid)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if err == nil {
		for _, shopID := range showcaseShopIDs.ShopIds {
			allowed[shopID] = struct{}{}
		}
	}
	return allowed, nil
}

func renderShopItem(
	evaluator *viber.CommerceConditionEvaluator,
	definition *papi.ShopItem,
	purchase *umodel.UserShopPurchase,
	now time.Time,
) (*papi.ShopItem, bool, error) {
	visible, err := evaluator.Satisfied(definition.GetViewConditionId())
	if err != nil || !visible {
		return nil, visible, err
	}
	unlocked, err := evaluator.Satisfied(definition.GetUnlockConditionId())
	if err != nil {
		return nil, false, err
	}
	item := proto.Clone(definition).(*papi.ShopItem)
	item.Unlocked = unlocked
	applyShopStock(item, purchase, now)
	return item, true, nil
}

func applyShopStock(item *papi.ShopItem, purchase *umodel.UserShopPurchase, now time.Time) {
	resetShopPurchaseCountIfNeeded(purchase, item, now)
	if item.GetPurchaseLimit() > 0 {
		item.LeftCount = max(item.GetPurchaseLimit()-purchase.PurchaseCounts[item.GetId()], 0)
	} else {
		item.LeftCount = 0
	}
	switch item.GetResetTimingType() {
	case penum.ResetTimingType_ResetTimingType_Daily,
		penum.ResetTimingType_ResetTimingType_Weekly,
		penum.ResetTimingType_ResetTimingType_Monthly:
		item.NextResetTime = purchase.ResetTimes[item.GetId()]
	default:
		item.NextResetTime = 0
	}
}

func resetShopPurchaseCountIfNeeded(purchase *umodel.UserShopPurchase, item *papi.ShopItem, now time.Time) {
	next := shopNextResetTime(now, item.GetResetTimingType())
	if next == 0 {
		delete(purchase.ResetTimes, item.GetId())
		return
	}
	if purchase.ResetTimes[item.GetId()] != next {
		purchase.PurchaseCounts[item.GetId()] = 0
		purchase.ResetTimes[item.GetId()] = next
	}
}

func shopNextResetTime(now time.Time, timing penum.ResetTimingType) int64 {
	switch timing {
	case penum.ResetTimingType_ResetTimingType_Daily:
		return nextJSTDayStart(now).UnixMilli()
	case penum.ResetTimingType_ResetTimingType_Weekly:
		return nextJSTWeekStart(now).UnixMilli()
	case penum.ResetTimingType_ResetTimingType_Monthly:
		return nextJSTMonthStart(now).UnixMilli()
	default:
		return 0
	}
}
