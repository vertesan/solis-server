package user

import (
	"context"
	"vertesan/solis-server/db/crud/opuser"
	"vertesan/solis-server/db/transaction"
)

func DeleteUser(suid string) error {
	err := transaction.WithTransaction(func(ctx context.Context) (interface{}, error) {
		if err := opuser.DeleteUserAuth(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUser(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserItems(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserCards(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserCharacters(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserCostumes(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserDecks(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserDeckPositions(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserPublic(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserPoints(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserStaffs(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserMessages(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserProfile(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserEmblems(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserBalance(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserTotalCount(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserPhotoReport(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserNotifications(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserHierarchy(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserTutorials(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserHomePositions(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserBuddy(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserHairs(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserCompany(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserShowcaseLike(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserPhotoPoses(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserGachaInfos(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserFanEventInfo(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserPromotionInfo(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserRefreshInfo(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserLessonProgress(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserExchangeBooth(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserNotiStoneShopItem(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserNotiNormalShopItem(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserEventInfo(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserHierarchyNotiInfo(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserEventStoryInfos(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserPhotoActivities(ctx, suid); err != nil {
			return nil, err
		}

		return nil, nil
	}, nil, nil)
	if err != nil {
		return err
	}
	return nil
}
