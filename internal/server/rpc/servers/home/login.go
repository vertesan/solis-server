package home

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *HomeServer) Login(ctx context.Context, req *papi.HomeLoginRequest) (*papi.HomeLoginResponse, error) {
	suid := viber.GetSuid(ctx).String()
	now := time.Now()
	var response *papi.HomeLoginResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updatedData := &papi.UpdatedData{}
		balance, err := opuser.FindUserBalance(ctx, suid)
		if err != nil {
			return nil, err
		}

		user, err := opuser.FindAndUpdateUser(
			ctx,
			suid,
			bson.M{"$set": bson.M{
				"lastLoginTime": now.UnixMilli(),
			}},
		)
		if err != nil {
			return nil, err
		}

		isFirstOfDay := !viber.SameJSTDay(user.LastLoginTime, now.UnixMilli())

		user.LastLoginTime = now.UnixMilli()
		updatedData.User = user

		if isFirstOfDay {
			totalCount, err := opuser.FindAndUpdateUserTotalCount(
				ctx,
				suid,
				bson.M{"$inc": bson.M{
					"loginCount": 1,
				}},
				options.FindOneAndUpdate().SetReturnDocument(options.After),
			)
			if err != nil {
				return nil, err
			}
			updatedData.TotalCount = totalCount

			loginBonusNotification := &ptransaction.UserNotification{
				NotificationType: penum.NotificationType_NotificationType_LoginBonus,
				Valid:            true,
			}
			if err := opuser.UpsertUserNotification(ctx, suid, loginBonusNotification); err != nil {
				return nil, err
			}
			updatedData.Notifications = append(updatedData.Notifications, loginBonusNotification)
		}
		purchasedLoginBonusShopInfos, err := applyPurchasedLoginBonusShops(
			ctx,
			suid,
			now.UnixMilli(),
			updatedData,
		)
		if err != nil {
			return nil, err
		}

		response = &papi.HomeLoginResponse{
			PurchasedLoginBonusShopInfos: purchasedLoginBonusShopInfos,
			CommonResponse:               viber.NewCommonResponse(updatedData),
		}
		profile, err := opuser.FindUserProfile(ctx, suid)
		if err != nil {
			return nil, err
		}
		birthdayEvent, err := birthday.Reconcile(ctx, suid, profile, now)
		if err != nil {
			return nil, err
		}
		response.BirthdayAdvInfo = birthdayEvent.NextAdv()
		response.BirthdayRemindInfo = birthdayEvent.Reminder()
		if err := salary.RecordAttendance(ctx, suid, now); err != nil {
			return nil, err
		}
		if _, err := viber.ReconcileUserMissions(ctx, suid, now.UnixMilli(), updatedData); err != nil {
			return nil, err
		}
		if balance.PaidBalance > 0 {
			response.PaidJpy = &papi.PaidJpy{Amount: balance.PaidBalance}
		}
		return response, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// applyPurchasedLoginBonusShops grants at most one scheduled reward per
// active package and JST day. After the last configured day, the permanent
// private-server package starts a new reward cycle.
func applyPurchasedLoginBonusShops(
	ctx context.Context,
	suid string,
	nowMillis int64,
	updated *papi.UpdatedData,
) ([]*papi.PurchasedLoginBonusShopInfo, error) {
	if nowMillis <= 0 || updated == nil {
		return nil, status.Error(codes.Internal, "login bonus grant context is invalid")
	}
	entries, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		return nil, err
	}
	purchase, err := viber.FindUserShopPurchaseOrEmpty(ctx, suid)
	if err != nil {
		return nil, err
	}
	purchasedInfos := make([]*papi.PurchasedLoginBonusShopInfo, 0)
	changed := false
	for _, entry := range entries {
		if entry.LoginBonusPackageItem == nil || !viber.LoginBonusShopIsActive(purchase, entry) {
			continue
		}
		bonuses := entry.LoginBonusPackageItem.GetLoginBonuses()
		if len(bonuses) == 0 {
			return nil, status.Error(codes.FailedPrecondition, "login bonus shop has no daily rewards")
		}
		progress := purchase.LoginBonusProgress[entry.ShopId]
		if progress < 0 || int(progress) > len(bonuses) {
			return nil, status.Error(codes.FailedPrecondition, "login bonus shop progress is invalid")
		}
		lastGrantTime := purchase.LoginBonusLastGrantTimes[entry.ShopId]
		if lastGrantTime == 0 || lastGrantTime < nowMillis && !viber.SameJSTDay(lastGrantTime, nowMillis) {
			nextProgress := progress + 1
			if int(nextProgress) > len(bonuses) {
				nextProgress = 1
			}
			bonus := findLoginBonusDay(bonuses, nextProgress)
			if bonus == nil || bonus.GetRewardId() == "" {
				return nil, status.Error(codes.FailedPrecondition, "login bonus shop daily reward is missing")
			}
			resources, err := viber.ResolveRewardResources(ctx, bonus.GetRewardId(), 1)
			if err != nil {
				return nil, err
			}
			if err := viber.ValidateExchangeRewardResources(resources); err != nil {
				return nil, err
			}
			for _, resource := range resources {
				if _, err := viber.GrantExchangeRewards(ctx, suid, resource, updated); err != nil {
					return nil, err
				}
			}
			purchase.LoginBonusProgress[entry.ShopId] = nextProgress
			purchase.LoginBonusLastGrantTimes[entry.ShopId] = nowMillis
			changed = true
		}
		info, loginPackage, err := viber.RenderLoginBonusShop(entry, purchase)
		if err != nil {
			return nil, err
		}
		purchasedInfos = append(purchasedInfos, &papi.PurchasedLoginBonusShopInfo{
			ShopInfo:              info,
			LoginBonusPackageItem: loginPackage,
		})
	}
	if changed {
		if err := opuser.UpsertUserShopPurchase(ctx, purchase); err != nil {
			return nil, err
		}
	}
	return purchasedInfos, nil
}

func findLoginBonusDay(bonuses []*papi.ShopLoginBonus, day int32) *papi.ShopLoginBonus {
	for _, bonus := range bonuses {
		if bonus.GetDay() == day {
			return bonus
		}
	}
	return nil
}
