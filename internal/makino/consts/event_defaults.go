package consts

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
)

var UserInitialEventInfo = &papi.HomeEventInfo{
	DreamInfo: &papi.HomeDreamInfo{
		Name:                    "イメージトレーニング",
		StartTime:               1776222000000,
		EndTime:                 4080557703000,
		AssetId:                 "event-dream-defalut",
		BannerAssetId:           "img_banner_l_event-dream-defalut",
		Order:                   60,
		UnlockConditionId:       "cd-dream-25-0320-play",
		IsUnlocked:              true,
		GenerateAreaItemId:      "item-dream-generate-dream-25-0320",
		IsReceivableLoginReward: true,
		EventMissionId:          "event-dream-26-04",
	},
}
