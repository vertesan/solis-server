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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func (s *HomeServer) SetCharacterPosition(
	ctx context.Context,
	req *papi.HomeSetCharacterPositionRequest,
) (*papi.HomeSetCharacterPositionResponse, error) {
	positions, err := homePositionsFromRequest(req)
	if err != nil {
		return nil, err
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.HomeSetCharacterPositionResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		var randomCharacterIDs []string
		for _, position := range positions {
			if !position.GetIsCharacterRandom() {
				continue
			}
			characters, err := opuser.FindAllUserCharacters(ctx, suid)
			if err != nil {
				return nil, err
			}
			for _, character := range characters {
				if character.GetCharacterId() != "" {
					randomCharacterIDs = append(randomCharacterIDs, character.GetCharacterId())
				}
			}
			break
		}
		for _, position := range positions {
			if position.GetIsCharacterRandom() {
				if len(randomCharacterIDs) == 0 {
					return nil, status.Error(codes.FailedPrecondition, "no owned character is available for a random home position")
				}
				index, err := viber.CryptoRandomIndex(len(randomCharacterIDs))
				if err != nil {
					return nil, err
				}
				position.CharacterId = randomCharacterIDs[index]
				continue
			}
			if _, err := opuser.FindUserCharacter(ctx, suid, position.GetCharacterId()); err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, status.Errorf(codes.PermissionDenied, "character %q is not owned", position.GetCharacterId())
				}
				return nil, err
			}
		}
		if err := opuser.UpsertManyUserHomePositions(ctx, suid, positions); err != nil {
			return nil, err
		}
		response = &papi.HomeSetCharacterPositionResponse{
			CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{HomePositions: positions}),
		}
		return nil, nil
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, status.Error(codes.FailedPrecondition, "home positions changed concurrently")
		}
		return nil, err
	}
	return response, nil
}

func homePositionsFromRequest(req *papi.HomeSetCharacterPositionRequest) ([]*ptransaction.UserHomePosition, error) {
	if req == nil || len(req.GetPositions()) != 3 {
		return nil, status.Error(codes.InvalidArgument, "home, training, and activity positions are required")
	}
	requiredTypes := map[penum.HomePositionType]bool{
		penum.HomePositionType_HomePositionType_Home:     false,
		penum.HomePositionType_HomePositionType_Training: false,
		penum.HomePositionType_HomePositionType_Activity: false,
	}
	positions := make([]*ptransaction.UserHomePosition, 0, len(req.GetPositions()))
	for _, requested := range req.GetPositions() {
		if requested == nil {
			return nil, status.Error(codes.InvalidArgument, "position must not be nil")
		}
		positionType := requested.GetType()
		seen, valid := requiredTypes[positionType]
		if !valid {
			return nil, status.Error(codes.InvalidArgument, "unknown home position type")
		}
		if seen {
			return nil, status.Error(codes.InvalidArgument, "home position types must be unique")
		}
		requiredTypes[positionType] = true
		if requested.GetIsCharacterRandom() && requested.GetCharacterId() != "" {
			return nil, status.Error(codes.InvalidArgument, "random position must not specify a character")
		}
		if !requested.GetIsCharacterRandom() && requested.GetCharacterId() == "" {
			return nil, status.Error(codes.InvalidArgument, "fixed position requires a character")
		}
		positions = append(positions, &ptransaction.UserHomePosition{
			HomePositionType:  positionType,
			CharacterId:       requested.GetCharacterId(),
			IsCharacterRandom: requested.GetIsCharacterRandom(),
			IsCostumeRandom:   requested.GetIsCostumeRandom(),
		})
	}
	return positions, nil
}

func (s *HomeServer) Enter(ctx context.Context, req *papi.HomeEnterRequest) (*papi.HomeEnterResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.HomeEnterResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		updatedData := &papi.UpdatedData{}
		gachaInfos, err := viber.ReconcileCurrentGachaInfos(ctx, suid, now)
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
		if err := viber.ReconcileActivityInfos(ctx, suid, now, fanEventInfo, promotionInfo, refreshInfo); err != nil {
			return nil, err
		}
		if err := viber.ReconcileActiveFanEventForHome(ctx, suid, now, fanEventInfo); err != nil {
			return nil, err
		}
		fanEventInfoForResponse := viber.ActivityFanEventInfoForResponse(fanEventInfo)
		promotionInfoForResponse := viber.ActivityPromotionInfoForResponse(promotionInfo)
		refreshInfoForResponse, err := viber.ActivityRefreshInfoForResponse(refreshInfo)
		if err != nil {
			return nil, err
		}
		lessonProgress, err := viber.ReconcileActivityLessonProgress(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		lessonInfo, err := viber.ReconcileActivityLessonInfo(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		lessonInfo.LessonProgress = lessonProgress
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
		if err := viber.ReconcileNarrativeSchedules(ctx, suid, now, updatedData); err != nil {
			return nil, err
		}

		response = &papi.HomeEnterResponse{
			GachaInfos:              gachaInfos,
			FanEventInfo:            fanEventInfoForResponse,
			PromotionInfo:           promotionInfoForResponse,
			RefreshInfo:             refreshInfoForResponse,
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
			LessonInfo:              lessonInfo,
			EventButtonDisplayInfos: []*papi.EventButtonDisplayInfo{}, // TODO
			ExtraStoryPartInfos:     extraStoryPartInfos,
			SpecialHomeActionInfos:  []*papi.SpecialHomeActionInfo{}, // TODO
			NotiShowcaseShopItem:    notiShowcaseShopItem,
			ShowcaseToyShopIds:      visibleShowcaseShopIds,
			UnlockedShowcaseNumbers: unlockedShowcaseNumber.Numbers,
			PhotoExpressions:        photoExpressions,
			CommonResponse:          viber.NewCommonResponse(updatedData),
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
