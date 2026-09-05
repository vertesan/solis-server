package viber

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func PhotoSetting(ctx context.Context) (*pmaster.Setting, error) {
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	return opmaster.FindSetting(ctx, settingID)
}

func DedupePhotoUpdatedItems(updated *papi.UpdatedData) {
	type itemKey struct {
		id          string
		expiredTime int64
	}
	last := make(map[itemKey]int, len(updated.GetItems()))
	for index, item := range updated.GetItems() {
		last[itemKey{id: item.GetItemId(), expiredTime: item.GetExpiredTime()}] = index
	}
	items := make([]*ptransaction.UserItem, 0, len(last))
	for index, item := range updated.GetItems() {
		if last[itemKey{id: item.GetItemId(), expiredTime: item.GetExpiredTime()}] == index {
			items = append(items, item)
		}
	}
	updated.Items = items
}
