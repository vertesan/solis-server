package viber

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// FindUserShopPurchaseOrEmpty returns a writable shop state, including the
// fields added after the original shop implementation. This also keeps older
// users whose document predates login packages usable without a data rewrite.
func FindUserShopPurchaseOrEmpty(ctx context.Context, suid string) (*umodel.UserShopPurchase, error) {
	purchase, err := opuser.FindUserShopPurchase(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		purchase = &umodel.UserShopPurchase{SUID: suid}
	}
	if purchase.PurchaseCounts == nil {
		purchase.PurchaseCounts = map[string]int32{}
	}
	if purchase.ResetTimes == nil {
		purchase.ResetTimes = map[string]int64{}
	}
	if purchase.PurchasedItemIds == nil {
		purchase.PurchasedItemIds = []string{}
	}
	if purchase.LoginBonusProgress == nil {
		purchase.LoginBonusProgress = map[string]int32{}
	}
	if purchase.LoginBonusLastGrantTimes == nil {
		purchase.LoginBonusLastGrantTimes = map[string]int64{}
	}
	return purchase, nil
}

func LoginBonusShopIsActive(purchase *umodel.UserShopPurchase, entry *smodel.ShopCatalogEntry) bool {
	if purchase == nil || entry == nil || entry.LoginBonusPackageItem == nil {
		return false
	}
	itemID := entry.LoginBonusPackageItem.GetShopItem().GetId()
	if itemID == "" {
		return false
	}
	for _, purchasedID := range purchase.PurchasedItemIds {
		if purchasedID == itemID {
			return true
		}
	}
	return false
}

// RenderLoginBonusShop derives all purchase/progress fields from the user's
// entitlement instead of storing those fields in the server-wide catalog.
func RenderLoginBonusShop(
	entry *smodel.ShopCatalogEntry,
	purchase *umodel.UserShopPurchase,
) (*papi.ShopInfo, *papi.LoginBonusPackageItem, error) {
	if purchase == nil || entry == nil || entry.Info == nil || entry.LoginBonusPackageItem == nil ||
		entry.LoginBonusPackageItem.GetShopItem() == nil {
		return nil, nil, status.Error(codes.FailedPrecondition, "login bonus shop catalog is incomplete")
	}
	bonuses := entry.LoginBonusPackageItem.GetLoginBonuses()
	if len(bonuses) == 0 {
		return nil, nil, status.Error(codes.FailedPrecondition, "login bonus shop has no daily rewards")
	}
	progress := purchase.LoginBonusProgress[entry.ShopId]
	if progress < 0 || int(progress) > len(bonuses) {
		return nil, nil, status.Error(codes.FailedPrecondition, "login bonus shop progress is invalid")
	}
	active := LoginBonusShopIsActive(purchase, entry)
	info := proto.Clone(entry.Info).(*papi.ShopInfo)
	info.IsPurchased = active
	info.Progress = progress
	info.MaxProgress = int32(len(bonuses))
	loginPackage := proto.Clone(entry.LoginBonusPackageItem).(*papi.LoginBonusPackageItem)
	loginPackage.IsPurchased = active
	for _, bonus := range loginPackage.LoginBonuses {
		bonus.IsReceived = active && bonus.GetDay() > 0 && bonus.GetDay() <= progress
	}
	return info, loginPackage, nil
}
