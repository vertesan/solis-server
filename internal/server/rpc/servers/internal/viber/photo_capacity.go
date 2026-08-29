package viber

import (
	"context"
	"fmt"
	"math"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
)

const PhotoCapacityGiftMessage = "所持上限を超えたためギフトに送られました"

func PhotoCapacityLimit(setting *pmaster.Setting, user *ptransaction.User) (int64, error) {
	if setting == nil || user == nil || setting.GetPhotoDefaultLimit() <= 0 ||
		setting.GetPhotoLimitExtendAmount() <= 0 || setting.GetPhotoLimitExtendLimit() < 0 ||
		user.GetPhotoLimitExtendedCount() < 0 {
		return 0, fmt.Errorf("photo capacity data is invalid")
	}
	extendedCount := min(user.GetPhotoLimitExtendedCount(), setting.GetPhotoLimitExtendLimit())
	base := int64(setting.GetPhotoDefaultLimit())
	extendAmount := int64(setting.GetPhotoLimitExtendAmount())
	if int64(extendedCount) > (math.MaxInt64-base)/extendAmount {
		return 0, fmt.Errorf("photo capacity overflows")
	}
	return base + int64(extendedCount)*extendAmount, nil
}

func FindPhotoCapacity(ctx context.Context, suid string) (owned int64, limit int64, err error) {
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return 0, 0, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return 0, 0, err
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return 0, 0, err
	}
	limit, err = PhotoCapacityLimit(setting, user)
	if err != nil {
		return 0, 0, err
	}
	photos, err := opuser.FindAllUserPhotos(ctx, suid)
	if err != nil {
		return 0, 0, err
	}
	return int64(len(photos)), limit, nil
}

func FindPhotoRecipeCapacity(ctx context.Context, suid string) (owned int64, limit int64, err error) {
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return 0, 0, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return 0, 0, err
	}
	limit = setting.GetPhotoRecipeLimit()
	if limit <= 0 {
		return 0, 0, fmt.Errorf("photo recipe capacity is invalid")
	}
	recipes, err := opuser.FindAllUserPhotoRecipes(ctx, suid)
	if err != nil {
		return 0, 0, err
	}
	for _, recipe := range recipes {
		if recipe.GetAmount() < 0 || owned > math.MaxInt64-recipe.GetAmount() {
			return 0, 0, fmt.Errorf("photo recipe amount is invalid")
		}
		owned += recipe.GetAmount()
	}
	return owned, limit, nil
}
