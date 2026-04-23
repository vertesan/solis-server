package home

import (
	"context"
	"vertesan/solis-server/db/crud/opsolis"
	"vertesan/solis-server/db/crud/opuser"
	"vertesan/solis-server/proto/papi"
)

func Enter(ctx context.Context, suid string) (*papi.HomeEnterResponse, error) {
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
		NotiShowcaseShopItem:    notiShowcaseShopItem,
		EventInfo:               eventInfo,
		HierarchyNotiInfo:       hierarchyNotiInfo,
		EventStoryInfos:         eventStoryInfos,
		PhotoActivities:         photoActivities,
		PhotoMusics:             photoMusics,
		PhotoStages:             photoStages,
		ExtraStoryPartInfos:     extraStoryPartInfos,
		PhotoExpressions:        photoExpressions,
		ShowcaseToyShopIds:      showcaseToyShopIds.ShopIds,
		UnlockedShowcaseNumbers: unlockedShowcaseNumber.Numbers,
		CommonResponse: &papi.CommonResponse{
			UpdatedData: &papi.UpdatedData{},
			DeletedData: &papi.DeletedData{},
		},
	}, nil
}
