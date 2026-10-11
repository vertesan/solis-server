package love

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func loadLove(ctx context.Context, suid, loveID string) (*moshikoi.Catalog, *umodel.UserLoveState, *ptransaction.UserLove, error) {
	catalog, err := moshikoi.LoadCatalog(ctx, loveID)
	if err != nil {
		return nil, nil, nil, err
	}
	state, err := moshikoi.LoadState(ctx, suid, loveID)
	if err != nil {
		return nil, nil, nil, err
	}
	if !state.Registered {
		return nil, nil, nil, viber.FailedPreconditionf("register for the love event first")
	}
	love, err := moshikoi.UserLove(ctx, suid, loveID)
	if err != nil {
		return nil, nil, nil, err
	}
	return catalog, state, love, nil
}

func storeLove(ctx context.Context, state *umodel.UserLoveState, love *ptransaction.UserLove, updated *papi.UpdatedData) error {
	if err := moshikoi.StoreState(ctx, state, love); err != nil {
		return err
	}
	updated.Loves = append(updated.Loves, moshikoi.VisibleLove(love))
	return viber.ReconcileLoveMissions(ctx, state.SUID, state.LoveID, state.Exp, updated)
}

func validateLoveName(name string) error {
	if !utf8.ValidString(name) || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 10 {
		return viber.InvalidArgumentf("love player name must contain 1 to 10 characters")
	}
	return nil
}

func loadLoveStory(ctx context.Context, suid, episodeID string) (*moshikoi.Catalog, *umodel.UserLoveState, *ptransaction.UserLove, *moshikoi.Routes, error) {
	for _, loveID := range moshikoi.SupportedLoveIDs() {
		if !strings.HasPrefix(episodeID, moshikoi.EpisodeIDPrefix(loveID)) {
			continue
		}
		catalog, state, love, err := loadLove(ctx, suid, loveID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		routes, err := moshikoi.LoadRoutes(loveID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if catalog.Episode(episodeID) == nil || routes.Episode(episodeID) == nil {
			return nil, nil, nil, nil, status.Error(codes.NotFound, "love story episode does not exist")
		}
		return catalog, state, love, routes, nil
	}
	return nil, nil, nil, nil, status.Error(codes.NotFound, "love story episode does not exist")
}

func loveHintLevel(catalog *moshikoi.Catalog, exp int64, requested int32) (int32, error) {
	var unlocked int32
	for hint, level := range catalog.Config.HintUnlockLevels {
		if catalog.Level(exp) >= level && hint > unlocked {
			unlocked = hint
		}
	}
	if requested < 0 || requested > unlocked {
		return 0, viber.InvalidArgumentf("love hint level is not unlocked")
	}
	return unlocked, nil
}

func startLoveStory(ctx context.Context, suid, episodeID string, hintLevel int32, restart bool) (*papi.LoveStartResponse, error) {
	var response *papi.LoveStartResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, state, love, routes, err := loadLoveStory(ctx, suid, episodeID)
		if err != nil {
			return nil, err
		}
		unlockedHint, err := loveHintLevel(catalog, state.Exp, hintLevel)
		if err != nil {
			return nil, err
		}
		episode := catalog.Episode(episodeID)
		if !slices.Contains(state.StoryEpisodeIDs, episodeID) {
			return nil, viber.FailedPreconditionf("story episode is not on the current route")
		}
		level, ok := catalog.Config.EpisodeUnlockLevels[episode.GetEpisode()]
		if !ok || catalog.Level(state.Exp) < level {
			return nil, viber.FailedPreconditionf("love level is too low for this episode")
		}
		if restart && (!love.IsReadingStory || state.ActiveStoryEpisodeID != episodeID) {
			return nil, viber.FailedPreconditionf("story episode is not being read")
		}
		if love.IsReadingStory && state.ActiveStoryEpisodeID != episodeID {
			return nil, viber.FailedPreconditionf("another story episode is being read")
		}
		updated := &papi.UpdatedData{}
		if !restart && !love.IsReadingStory {
			if amount := episode.GetConsumptionItemAmount(); amount > 0 && !routes.AllEndingsUnlocked(love.UnlockedLoveStoryEndIDs) {
				if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, catalog.Config.StoryReadItemID, int64(amount), updated); err != nil {
					if errors.Is(err, viber.ErrExchangeInsufficientResource) {
						return nil, status.Error(codes.ResourceExhausted, "love story points are insufficient")
					}
					return nil, err
				}
			}
			love.IsReadingStory = true
			state.ActiveStoryEpisodeID = episodeID
			state.LastPlayTime = time.Now().UnixMilli()
			if err := storeLove(ctx, state, love, updated); err != nil {
				return nil, err
			}
		}
		response = &papi.LoveStartResponse{StoryChoiceGroupInfos: routes.ChoiceInfos(episodeID, state.PastSelections, unlockedHint), AdvBranchInfos: routes.BranchInfos(episodeID, state.Selections), CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}

func validateLoveSave(ctx context.Context, love *ptransaction.UserLove, slot int32) error {
	if len(love.UnlockedLoveStoryEndIDs) == 0 {
		return viber.FailedPreconditionf("clear an ending to unlock save slots")
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return err
	}
	if slot < 1 || slot > setting.GetLoveSaveSlotNum() {
		return viber.InvalidArgumentf("invalid love save slot")
	}
	return nil
}

func adjustLoveFavorability(love *ptransaction.UserLove, delta int64) error {
	if delta > 0 && love.HiddenFavorability > math.MaxInt64-delta || delta < 0 && love.HiddenFavorability < math.MinInt64-delta {
		return status.Error(codes.OutOfRange, "love favorability overflows")
	}
	love.HiddenFavorability += delta
	return nil
}

func mergeLoveMissions(updated *papi.UpdatedData, missions []*ptransaction.UserMission) {
	for _, mission := range missions {
		found := false
		for i, prior := range updated.Missions {
			if prior.GetMissionId() == mission.GetMissionId() {
				updated.Missions[i] = mission
				found = true
				break
			}
		}
		if !found {
			updated.Missions = append(updated.Missions, mission)
		}
	}
}

func receiveLoveLesson(ctx context.Context, suid, loveID string, promote bool) (*papi.LoveReceiveLessonRewardResponse, error) {
	catalog, state, love, err := loadLove(ctx, suid, loveID)
	if err != nil {
		return nil, err
	}
	updated := &papi.UpdatedData{}
	if err := viber.ReconcileLoveMissions(ctx, suid, loveID, state.Exp, updated); err != nil {
		return nil, err
	}
	if promote {
		if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, catalog.Config.PromoteItemID, 1, updated); err != nil {
			if errors.Is(err, viber.ErrExchangeInsufficientResource) {
				return nil, status.Error(codes.ResourceExhausted, "love skip drink is insufficient")
			}
			return nil, err
		}
		missions, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_ItemUsedAmount, 1, catalog.Config.PromoteItemID)
		if err != nil {
			return nil, err
		}
		mergeLoveMissions(updated, missions)
	}
	exp, points, err := catalog.LessonReward(state, time.Now().UnixMilli(), promote)
	if err != nil {
		return nil, err
	}
	rewards := []*ptransaction.Reward{}
	if points > 0 {
		if _, err := viber.GrantExchangeItem(ctx, suid, catalog.Config.StoryReadItemID, points, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, &ptransaction.Reward{ResourceType: penum.ResourceType_ResourceType_Item, ResourceId: catalog.Config.StoryReadItemID, Amount: points})
	}
	if err := storeLove(ctx, state, love, updated); err != nil {
		return nil, err
	}
	return &papi.LoveReceiveLessonRewardResponse{Rewards: rewards, ObtainExp: exp, TotalExp: state.Exp, LessonProgress: catalog.LessonProgress(state), CommonResponse: viber.NewCommonResponse(updated)}, nil
}
