package servers

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/internal/viber"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type HomeServer struct {
	pb.UnimplementedHomeServer
}

var jstZone = time.FixedZone("JST", 9*60*60)

func (s *HomeServer) Login(ctx context.Context, req *pb.HomeLoginRequest) (*pb.HomeLoginResponse, error) {
	suid := viber.GetSuid(ctx).String()
	now := time.Now()
	var response *papi.HomeLoginResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updatedData := &papi.UpdatedData{}

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

		prevLogin := time.UnixMilli(user.LastLoginTime).In(jstZone)
		nowJst := now.In(jstZone)
		isFirstOfDay := prevLogin.Year() != nowJst.Year() ||
			prevLogin.YearDay() != nowJst.YearDay()

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
		}

		response = &papi.HomeLoginResponse{
			CommonResponse: &papi.CommonResponse{
				UpdatedData: updatedData,
				DeletedData: &papi.DeletedData{},
			},
		}
		return response, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *HomeServer) Enter(ctx context.Context, req *pb.HomeEnterRequest) (*pb.HomeEnterResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.HomeEnterResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		gachaInfos, err := opuser.FindAllUserGachaInfos(ctx, suid)
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
		eventInfo, err := opuser.FindUserEventInfo(ctx, suid)
		if err != nil {
			return nil, err
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

		return &papi.HomeEnterResponse{
			GachaInfos:              gachaInfos,
			FanEventInfo:            fanEventInfo,
			PromotionInfo:           promotionInfo,
			RefreshInfo:             refreshInfo,
			LessonProgress:          lessonProgress,
			BannerInfo:              &papi.BannerInfo{MenuBanners: menuBanners},
			ShopIds:                 shopIds.ShopIds,
			ExchangeBoothIds:        exchangeBooth.BoothIds,
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
			ShowcaseToyShopIds:      showcaseToyShopIds.ShopIds,
			UnlockedShowcaseNumbers: unlockedShowcaseNumber.Numbers,
			PhotoExpressions:        photoExpressions,
			CommonResponse: &papi.CommonResponse{
				UpdatedData: &papi.UpdatedData{},
				DeletedData: &papi.DeletedData{},
			},
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
