package love

import (
	"context"
	"slices"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/protobuf/proto"
)

func (s *LoveServer) Top(ctx context.Context, req *papi.LoveTopRequest) (*papi.LoveTopResponse, error) {
	var response *papi.LoveTopResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		suid := viber.GetSuid(ctx).String()
		catalog, state, love, err := loadLove(ctx, suid, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		state.LastPlayTime = time.Now().UnixMilli()
		updated := &papi.UpdatedData{}
		if err := storeLove(ctx, state, love, updated); err != nil {
			return nil, err
		}
		info, err := viber.BuildLoveInfo(ctx, suid, catalog, state.LastPlayTime)
		if err != nil {
			return nil, err
		}
		normal, limited, err := loveEventMissionInfos(ctx, suid, catalog.LoveID)
		if err != nil {
			return nil, err
		}
		save := &papi.LoveSaveInfo{Unlocked: len(love.GetUnlockedLoveStoryEndIDs()) > 0, SaveDataInfos: []*papi.LoveSaveDataInfo{}}
		for _, slot := range state.Saves {
			save.SaveDataInfos = append(save.SaveDataInfos, slot.Info)
		}
		levelInfos := []*papi.LoveLevelInfo{}
		current := catalog.Level(state.Exp)
		for _, master := range catalog.Levels {
			// The official UI reveals its level roadmap five rows at a time,
			// including the next level's text before it becomes reachable.
			if master.GetLevel() > ((current+5)/5)*5 {
				continue
			}
			entry := &papi.LoveLevelInfo{Level: master.GetLevel(), IsTextViewable: master.GetLevel() <= current+1}
			for episode, level := range catalog.Config.EpisodeUnlockLevels {
				if level == entry.Level {
					entry.UnlockStoryEpisode = episode
				}
			}
			for hint, level := range catalog.Config.HintUnlockLevels {
				if level == entry.Level {
					entry.UnlockLoveHintType = penum.LoveHintType(hint)
				}
			}
			levelInfos = append(levelInfos, entry)
		}
		response = &papi.LoveTopResponse{
			LoveName:        catalog.Config.Name,
			PlayerName:      state.PlayerName,
			Exp:             state.Exp,
			LessonProgress:  catalog.LessonProgress(state),
			StoryEpisodeIds: state.StoryEpisodeIDs,
			SaveInfo:        save,
			ItemInfo: &papi.LoveItemInfo{
				StoryReadItemId:                catalog.Config.StoryReadItemID,
				HiddenFavorabilityObtainItemId: catalog.Config.PresentItemID,
				LessonPromoteItemId:            catalog.Config.PromoteItemID,
			},
			EventMissionInfos:        normal,
			LimitedEventMissionInfos: limited,
			HiddenFavorability:       max(int64(0), love.GetHiddenFavorability()),
			StoryEndInfos:            info.StoryEndInfos,
			LoveLevelInfos:           levelInfos,
			BannerAssetId:            catalog.Config.BannerAssetID,
			ExpAssetId:               "exp-" + catalog.LoveID,
			MainCharacterInfo:        catalog.MainCharacterInfo(),
			LoveSimpleName:           "もし恋",
			CommonResponse:           viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	return response, err
}

func loveEventMissionInfos(ctx context.Context, suid, loveID string) ([]*papi.EventMissionInfo, []*papi.EventMissionInfo, error) {
	catalog, err := moshikoi.LoadCatalog(ctx, loveID)
	if err != nil {
		return nil, nil, err
	}
	groups, err := viber.LoveMissionGroups(ctx, catalog)
	if err != nil {
		return nil, nil, err
	}
	users, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]*ptransaction.UserMission, len(users))
	for _, user := range users {
		byID[user.GetMissionId()] = user
	}
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
	normal, limited := []*papi.EventMissionInfo{}, []*papi.EventMissionInfo{}
	for _, group := range groups {
		// The event is permanent. Bypass historical date gates only; retain card
		// ownership and other non-calendar gates (e.g. the costume mission group).
		unlocked := true
		conditionID := group.GetUnlockConditionId()
		if conditionID != "" {
			condition, err := opmaster.FindCondition(ctx, conditionID)
			if err != nil {
				return nil, nil, err
			}
			permanent := proto.Clone(condition).(*pmaster.Condition)
			for _, setting := range permanent.GetSettings() {
				setting.Term = nil
			}
			evaluator.PrimeConditions([]*pmaster.Condition{permanent})
			unlocked, err = evaluator.Satisfied(conditionID)
			if err != nil {
				return nil, nil, err
			}
		}
		info := &papi.EventMissionInfo{Id: group.GetId(), Unlocked: unlocked, UserMissions: []*ptransaction.UserMission{}}
		masters, err := opmaster.FindMissionsByIds(ctx, group.GetMissionIds())
		if err != nil {
			return nil, nil, err
		}
		masterByID := make(map[string]*pmaster.Mission, len(masters))
		for _, master := range masters {
			masterByID[master.GetId()] = master
		}
		for _, id := range group.GetMissionIds() {
			if user := byID[id]; user != nil {
				visible, err := evaluator.Satisfied(masterByID[id].GetViewConditionId())
				if err != nil {
					return nil, nil, err
				}
				if visible {
					info.UserMissions = append(info.UserMissions, user)
				}
			}
		}
		if slices.Contains(catalog.Config.LimitedEventMissionIDs, group.GetId()) {
			limited = append(limited, info)
		} else {
			normal = append(normal, info)
		}
	}
	return normal, limited, nil
}
