package servers

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type HomeServer struct {
	papi.UnimplementedHomeServer
}

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
		purchasedLoginBonusShopInfos, err := viber.ApplyPurchasedLoginBonusShops(
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

func (s *HomeServer) Enter(ctx context.Context, req *papi.HomeEnterRequest) (*papi.HomeEnterResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.HomeEnterResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		gachaInfos, err := viber.ReconcileCurrentGachaInfos(ctx, suid, time.Now().UnixMilli())
		if err != nil {
			return nil, err
		}
		fanEventInfo, err := opuser.FindUserFanEventInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		promotionInfo, err := opuser.FindUserPromotionInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		refreshInfo, err := opuser.FindUserRefreshInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		lessonProgress, err := opuser.FindUserLessonProgress(ctx, suid)
		if err != nil {
			return nil, err
		}
		shopIds, err := opuser.FindUserShopIds(ctx, suid)
		if err != nil {
			return nil, err
		}
		exchangeBooth, err := opuser.FindUserExchangeBooth(ctx, suid)
		if err != nil {
			return nil, err
		}
		notiStoneShopItem, err := opuser.FindUserNotiStoneShopItem(ctx, suid)
		if err != nil {
			return nil, err
		}
		notiNormalShopItem, err := opuser.FindUserNotiNormalShopItem(ctx, suid)
		if err != nil {
			return nil, err
		}
		notiShowcaseShopItem, err := opuser.FindUserNotiShowcaseShopItem(ctx, suid)
		if err != nil {
			return nil, err
		}
		eventInfo, err := opsolis.FindHomeEventInfo(ctx)
		if err != nil {
			return nil, err
		}
		godState, err := opuser.FindUserGodState(ctx, suid)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		if err == nil && godState.UnlockCurrentEvents {
			unlockHomeEventInfo(eventInfo.ProtoReflect())
		}
		hierarchyNotiInfo, err := opuser.FindUserHierarchyNotiInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		eventStoryInfos, err := opuser.FindAllUserEventStoryInfos(ctx, suid)
		if err != nil {
			return nil, err
		}
		photoActivities, err := opuser.FindAllUserPhotoActivities(ctx, suid)
		if err != nil {
			return nil, err
		}
		photoMusics, err := opuser.FindAllUserPhotoMusics(ctx, suid)
		if err != nil {
			return nil, err
		}
		photoStages, err := opuser.FindAllUserPhotoStages(ctx, suid)
		if err != nil {
			return nil, err
		}
		extraStoryPartInfos, err := opuser.FindAllUserExtraStoryPartInfos(ctx, suid)
		if err != nil {
			return nil, err
		}
		photoExpressions, err := opuser.FindAllUserPhotoExpressions(ctx, suid)
		if err != nil {
			return nil, err
		}
		showcaseToyShopIds, err := opuser.FindUserShowcaseToyShopIds(ctx, suid)
		if err != nil {
			return nil, err
		}
		unlockedShowcaseNumber, err := opuser.FindUserUnlockedShowcaseNumber(ctx, suid)
		if err != nil {
			return nil, err
		}
		menuBanners, err := opsolis.FindAllMenuBanners(ctx)
		if err != nil {
			return nil, err
		}
		userCurrentQuestArea, err := opuser.FindUserCurrentQuestArea(ctx, suid)
		if err != nil {
			return nil, err
		}
		visibleShopIds, visibleShowcaseShopIds, visibleExchangeBoothIds,
			allShopIds, allShowcaseShopIds, allExchangeBoothIds, err := viber.FilterHomeCommerceIDs(
			ctx,
			suid,
			shopIds.ShopIds,
			showcaseToyShopIds.ShopIds,
			exchangeBooth.BoothIds,
		)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserShopId(ctx, &umodel.UserShopId{SUID: suid, ShopIds: allShopIds}); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserShowcaseToyShopId(ctx, &umodel.UserShowcaseToyShopId{
			SUID: suid, ShopIds: allShowcaseShopIds,
		}); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserExchangeBooth(ctx, &umodel.UserExchangeBooth{
			SUID: suid, BoothIds: allExchangeBoothIds,
		}); err != nil {
			return nil, err
		}

		response = &papi.HomeEnterResponse{
			GachaInfos:              gachaInfos,
			FanEventInfo:            fanEventInfo,
			PromotionInfo:           promotionInfo,
			RefreshInfo:             refreshInfo,
			LessonProgress:          lessonProgress,
			BannerInfo:              &papi.BannerInfo{MenuBanners: menuBanners},
			ShopIds:                 visibleShopIds,
			ExchangeBoothIds:        visibleExchangeBoothIds,
			NotiStoneShopItem:       notiStoneShopItem,
			NotiNormalShopItem:      notiNormalShopItem,
			CurrentMainAreaId:       userCurrentQuestArea.CurrentMainAreaId,
			EventInfo:               eventInfo,
			HierarchyNotiInfo:       hierarchyNotiInfo,
			CurrentTowerQuestId:     userCurrentQuestArea.CurrentTowerQuestId,
			EventStoryInfos:         eventStoryInfos,
			PhotoActivities:         photoActivities,
			PhotoMusics:             photoMusics,
			PhotoStages:             photoStages,
			LessonInfo:              &papi.LessonInfo{LessonProgress: lessonProgress},
			EventButtonDisplayInfos: []*papi.EventButtonDisplayInfo{}, // TODO
			ExtraStoryPartInfos:     extraStoryPartInfos,
			SpecialHomeActionInfos:  []*papi.SpecialHomeActionInfo{}, // TODO
			NotiShowcaseShopItem:    notiShowcaseShopItem,
			ShowcaseToyShopIds:      visibleShowcaseShopIds,
			UnlockedShowcaseNumbers: unlockedShowcaseNumber.Numbers,
			PhotoExpressions:        photoExpressions,
			CommonResponse:          viber.NewCommonResponse(nil),
		}
		return response, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func unlockHomeEventInfo(message protoreflect.Message) {
	fields := message.Descriptor().Fields()
	for index := 0; index < fields.Len(); index++ {
		field := fields.Get(index)
		if field.Kind() == protoreflect.BoolKind && (field.JSONName() == "isUnlocked" || field.JSONName() == "unlocked") {
			message.Set(field, protoreflect.ValueOfBool(true))
			continue
		}
		if field.IsList() && field.Message() != nil {
			list := message.Get(field).List()
			for itemIndex := 0; itemIndex < list.Len(); itemIndex++ {
				unlockHomeEventInfo(list.Get(itemIndex).Message())
			}
			continue
		}
		if field.Message() != nil && message.Has(field) {
			unlockHomeEventInfo(message.Get(field).Message())
		}
	}
}
