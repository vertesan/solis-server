package exchange

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// nextJSTMonthlyOccurrence advances a configured JST reset timestamp to the
// first matching monthly occurrence strictly after now.
func nextJSTMonthlyOccurrence(resetMillis, nowMillis int64) int64 {
	reset := time.UnixMilli(resetMillis).In(viber.JstLocation)
	current := time.UnixMilli(nowMillis).In(viber.JstLocation)
	months := (current.Year()-reset.Year())*12 + int(current.Month()-reset.Month())
	next := reset.AddDate(0, months, 0)
	if !next.After(current) {
		next = next.AddDate(0, 1, 0)
	}
	return next.UnixMilli()
}

func findExchangePurchaseOrEmpty(ctx context.Context, suid string) (*umodel.UserExchangePurchase, error) {
	purchase, err := opuser.FindUserExchangePurchase(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		purchase = &umodel.UserExchangePurchase{SUID: suid}
	}
	if purchase.PurchaseCounts == nil {
		purchase.PurchaseCounts = map[string]int32{}
	}
	if purchase.ResetTimes == nil {
		purchase.ResetTimes = map[string]int64{}
	}
	return purchase, nil
}

func applyExchangeStock(booth *papi.ExchangeBooth, purchase *umodel.UserExchangePurchase) {
	for _, item := range booth.Exchanges {
		resetPurchaseCountIfNeeded(purchase, item)
		if item.ExchangeLimit > 0 {
			item.LeftCount = max(item.ExchangeLimit-purchase.PurchaseCounts[item.Id], 0)
		} else {
			item.LeftCount = 0
		}
	}
}

func resetPurchaseCountIfNeeded(purchase *umodel.UserExchangePurchase, item *papi.ExchangeItem) {
	if item.NextResetTime <= 0 || item.ResetTimingType == penum.ResetTimingType_ResetTimingType_Unknown ||
		item.ResetTimingType == penum.ResetTimingType_ResetTimingType_Never {
		return
	}

	resetTime := purchase.ResetTimes[item.Id]
	if resetTime == 0 || item.NextResetTime > resetTime {
		purchase.PurchaseCounts[item.Id] = 0
		resetTime = item.NextResetTime
		purchase.ResetTimes[item.Id] = resetTime
	}
	if now := time.Now().UnixMilli(); resetTime <= now {
		purchase.PurchaseCounts[item.Id] = 0
		purchase.ResetTimes[item.Id] = nextExchangeResetTime(resetTime, item.ResetTimingType, now)
	}
}

func nextExchangeResetTime(resetTime int64, timing penum.ResetTimingType, now int64) int64 {
	switch timing {
	case penum.ResetTimingType_ResetTimingType_Daily:
		const period = int64(24 * time.Hour / time.Millisecond)
		return resetTime + ((now-resetTime)/period+1)*period
	case penum.ResetTimingType_ResetTimingType_Weekly:
		const period = int64(7 * 24 * time.Hour / time.Millisecond)
		return resetTime + ((now-resetTime)/period+1)*period
	case penum.ResetTimingType_ResetTimingType_Monthly:
		return nextJSTMonthlyOccurrence(resetTime, now)
	default:
		return resetTime
	}
}
