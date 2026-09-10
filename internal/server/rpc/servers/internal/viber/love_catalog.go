package viber

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

// BuildLoveInfo is shared by the Home event entrance and Love title pages.
// It never registers a user merely because the event is visible.
func BuildLoveInfo(ctx context.Context, suid string, catalog *moshikoi.Catalog, now int64) (*papi.LoveInfo, error) {
	state, err := moshikoi.LoadState(ctx, suid, catalog.LoveID)
	if err != nil {
		return nil, err
	}
	love, err := moshikoi.UserLove(ctx, suid, catalog.LoveID)
	if err != nil {
		return nil, err
	}
	unlocked, err := NewCommerceConditionEvaluator(ctx, suid, now).Satisfied(catalog.Config.UnlockConditionID)
	if err != nil {
		return nil, err
	}
	items, err := opuser.FindAllUserItems(ctx, suid)
	if err != nil {
		return nil, err
	}
	hasPresent := false
	for _, item := range items {
		if item.GetItemId() == catalog.Config.PresentItemID && item.GetAmount() > 0 && (item.GetExpiredTime() == 0 || item.GetExpiredTime() > now) {
			hasPresent = true
		}
	}
	endings, err := moshikoi.LoadRoutes(catalog.LoveID)
	if err != nil {
		return nil, err
	}
	endInfos := []*papi.LoveStoryEndInfo{}
	if len(love.GetUnlockedLoveStoryEndIDs()) > 0 {
		for _, end := range endings.Endings {
			endInfos = append(endInfos, &papi.LoveStoryEndInfo{Id: end.ID, Name: end.Name, Unlocked: slices.Contains(love.GetUnlockedLoveStoryEndIDs(), end.ID)})
		}
	}
	return &papi.LoveInfo{
		LoveId:                          catalog.LoveID,
		Description:                     catalog.Config.Description,
		LastPlayTime:                    state.LastPlayTime,
		StoryEndInfos:                   endInfos,
		Type:                            penum.LoveType_LoveType_Event,
		StartTime:                       catalog.Config.StartTime,
		UnlockConditionId:               catalog.Config.UnlockConditionID,
		IsUnlocked:                      unlocked,
		AssetId:                         catalog.LoveID,
		EventMissionIds:                 catalog.Config.EventMissionIDs,
		LimitedEventMissionIds:          catalog.Config.LimitedEventMissionIDs,
		IsLessonRewardMax:               state.Registered && now >= catalog.LessonProgress(state).GetMaxRewardTime(),
		IsUnplayed:                      !state.Registered,
		HasHiddenFavorabilityObtainItem: hasPresent,
		Order:                           1,
		MainCharacterInfo:               catalog.MainCharacterInfo(),
		BaseType:                        penum.LoveBaseType_LoveBaseType_Standard,
	}, nil
}
