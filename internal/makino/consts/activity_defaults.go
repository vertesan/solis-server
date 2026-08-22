package consts

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

const beginnerActivityCampaignEndTime int64 = 4124271599000

func GetUserInitialLessonProgress() *papi.ActivityLessonProgress {
	return &papi.ActivityLessonProgress{
		StepRewardGold:              5,
		StepRewardManagerExp:        8,
		StepRewardCardEnhanceItem:   3,
		RewardGoldAmount:            0,
		RewardManagerExpAmount:      0,
		RewardCardEnhanceItemAmount: 0,
		LastReceiveTime:             0,
		MaxRewardTime:               0,
		CurrentAreaId:               "area-main-001",
		DailyPromoteCount:           0,
	}
}

func GetUserInitialLessonInfo() *papi.LessonInfo {
	return &papi.LessonInfo{
		CampaignEffects: []*papi.ActivityCampaignEffect{
			{
				Type:    penum.ActivityCampaignEffectType_ActivityCampaignEffectType_LessonExpUp,
				Value:   1000,
				EndTime: beginnerActivityCampaignEndTime,
			},
		},
		CampaignDropRewardIds: []string{},
	}
}

func GetUserInitialFanEventInfo() *papi.FanEventInfo {
	return &papi.FanEventInfo{
		FanEvents: []*papi.FanEvent{
			{
				Id:                         "act-fan-01",
				Name:                       "握手会",
				SubName:                    "握手会",
				Type:                       penum.ActivityFanEventType_ActivityFanEventType_Handshake,
				UnlockConditionId:          "cd-clear-area-main-007",
				ActivityPerformanceId:      "handshake-stage-1",
				Level:                      1,
				NextLevelRequiredExp:       800,
				RequiredStamina:            600,
				NextLevelUnlockConditionId: "cd-clear-area-main-008",
				StepConsumptionStamina:     8,
				FixedRewardId:              "rd-act-fan-01-lv1",
				CampaignDropRewardIds:      []string{},
				IsUnlocked:                 false,
				IsNextLevelUnlocked:        false,
			},
			{
				Id:                         "act-fan-02",
				Name:                       "撮影会",
				SubName:                    "撮影会",
				Type:                       penum.ActivityFanEventType_ActivityFanEventType_PhotoSession,
				UnlockConditionId:          "cd-clear-area-main-010",
				ActivityPerformanceId:      "photo-session-stage-1",
				Level:                      1,
				NextLevelRequiredExp:       2392,
				RequiredStamina:            800,
				NextLevelUnlockConditionId: "cd-clear-area-main-012",
				StepConsumptionStamina:     7,
				FixedRewardId:              "rd-act-fan-02-lv1",
				CampaignDropRewardIds:      []string{},
				IsUnlocked:                 false,
				IsNextLevelUnlocked:        false,
			},
			{
				Id:                         "act-fan-03",
				Name:                       "サイン会",
				SubName:                    "サイン会",
				Type:                       penum.ActivityFanEventType_ActivityFanEventType_Sign,
				UnlockConditionId:          "cd-clear-area-main-015",
				ActivityPerformanceId:      "sign-stage-1",
				Level:                      1,
				NextLevelRequiredExp:       8000,
				RequiredStamina:            1000,
				NextLevelUnlockConditionId: "cd-clear-area-main-018",
				StepConsumptionStamina:     5,
				FixedRewardId:              "rd-act-fan-03-lv1",
				CampaignDropRewardIds:      []string{},
				IsUnlocked:                 false,
				IsNextLevelUnlocked:        false,
			},
		},
	}
}

func GetUserInitialPromotionInfo() *papi.PromotionInfo {
	return &papi.PromotionInfo{
		CampaignEffects: []*papi.ActivityCampaignEffect{
			{
				Type:    penum.ActivityCampaignEffectType_ActivityCampaignEffectType_PromotionCardEnhanceItemUp,
				Value:   1000,
				EndTime: beginnerActivityCampaignEndTime,
			},
		},
		Promotions: []*papi.Promotion{
			{
				Id:                             "act-pro-01",
				Name:                           "宣伝",
				SubName:                        "チラシ配り",
				UnlockConditionId:              "cd-clear-area-main-003",
				ActivityPerformanceId:          "flyers",
				Level:                          1,
				NextLevelRequiredExp:           11,
				RequiredStamina:                1,
				NextLevelUnlockConditionId:     "cd-clear-area-main-003",
				StepConsumptionStamina:         1,
				StepRewardGold:                 1,
				StepRewardCardEnhanceItem:      4,
				MultiStepRewardId:              "rd-act-pro-01-01",
				MaxFanAmount:                   1000,
				SubGenre:                       "flyers",
				MultiStepCampaignDropRewardIds: []string{},
				IsUnlocked:                     false,
				IsNextLevelUnlocked:            false,
			},
			{
				Id:                             "act-pro-02",
				Name:                           "撮影",
				SubName:                        "PR写真撮影",
				UnlockConditionId:              "cd-clear-area-main-007",
				ActivityPerformanceId:          "pr_shooting",
				Level:                          1,
				NextLevelRequiredExp:           4296,
				RequiredStamina:                1,
				NextLevelUnlockConditionId:     "cd-clear-area-main-009",
				StepConsumptionStamina:         1,
				StepRewardGold:                 54,
				StepRewardCardEnhanceItem:      33,
				MultiStepRewardId:              "rd-act-pro-02-01",
				MaxFanAmount:                   14000,
				SubGenre:                       "pr-shooting",
				MultiStepCampaignDropRewardIds: []string{},
				IsUnlocked:                     false,
				IsNextLevelUnlocked:            false,
			},
			{
				Id:                             "act-pro-03",
				Name:                           "出演",
				SubName:                        "番組配信",
				UnlockConditionId:              "cd-clear-area-main-007",
				ActivityPerformanceId:          "video_distribution",
				Level:                          1,
				NextLevelRequiredExp:           4296,
				RequiredStamina:                1,
				NextLevelUnlockConditionId:     "cd-clear-area-main-009",
				StepConsumptionStamina:         1,
				StepRewardGold:                 54,
				StepRewardCardEnhanceItem:      33,
				MultiStepRewardId:              "rd-act-pro-03-01",
				MaxFanAmount:                   14000,
				SubGenre:                       "video-distribution",
				MultiStepCampaignDropRewardIds: []string{},
				IsUnlocked:                     false,
				IsNextLevelUnlocked:            false,
			},
		},
	}
}

func GetUserInitialRefreshInfo() *papi.RefreshInfo {
	return &papi.RefreshInfo{
		Refreshes: []*papi.Refresh{
			{
				Id:                   "act-ref-01",
				Name:                 "休息",
				RequiredMinutes:      120,
				UnlockConditionId:    "cd-clear-area-main-009",
				Level:                1,
				NextLevelRequiredExp: 1,
				Levels: []*papi.RefreshLevel{
					{
						Level:                 1,
						Name:                  "カフェ",
						RequiredGold:          60000,
						ActivityPerformanceId: "cafe",
						RecoveryAmount:        1000,
						SubGenre:              "cafe",
						IsUnlocked:            false,
					},
					{
						Level:                 2,
						Name:                  "ショッピング",
						RequiredGold:          210000,
						UnlockConditionId:     "cd-clear-area-main-021",
						ActivityPerformanceId: "shopping",
						RecoveryAmount:        1800,
						SubGenre:              "shopping",
						IsUnlocked:            false,
					},
				},
				IsUnlocked: false,
			},
			{
				Id:                   "act-ref-02",
				Name:                 "休暇",
				RequiredMinutes:      480,
				UnlockConditionId:    "cd-clear-area-main-014",
				Level:                1,
				NextLevelRequiredExp: 1,
				Levels: []*papi.RefreshLevel{
					{
						Level:                 1,
						Name:                  "寮で過ごす",
						RequiredGold:          130000,
						ActivityPerformanceId: "dormitory",
						RecoveryAmount:        2500,
						SubGenre:              "dormitory",
						IsUnlocked:            false,
					},
					{
						Level:                 2,
						Name:                  "テーマパーク",
						RequiredGold:          360000,
						UnlockConditionId:     "cd-clear-area-main-026",
						ActivityPerformanceId: "theme_park",
						RecoveryAmount:        3800,
						SubGenre:              "theme-park",
						IsUnlocked:            false,
					},
				},
				IsUnlocked: false,
			},
		},
	}
}
