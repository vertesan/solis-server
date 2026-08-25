package servers

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type StoryServer struct {
	papi.UnimplementedStoryServer
}

func (s *StoryServer) Read(ctx context.Context, req *papi.StoryReadRequest) (*papi.StoryReadResponse, error) {
	if req.GetStoryId() == "" {
		return nil, viber.InvalidArgumentf("storyId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.StoryReadResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		rewards, updated, err := readUserStory(ctx, suid, req.StoryId, false)
		if err != nil {
			return nil, err
		}
		response = &papi.StoryReadResponse{Rewards: rewards, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *StoryServer) ListEvent(ctx context.Context, _ *papi.Empty) (*papi.StoryListEventResponse, error) {
	suid := viber.GetSuid(ctx).String()
	infos, err := opuser.FindAllUserEventStoryInfos(ctx, suid)
	if err != nil {
		return nil, err
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Id < infos[j].Id })
	for _, info := range infos {
		sort.Slice(info.Episodes, func(i, j int) bool { return info.Episodes[i].Episode < info.Episodes[j].Episode })
	}
	return &papi.StoryListEventResponse{
		Infos:          infos,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *StoryServer) ListExtra(ctx context.Context, _ *papi.Empty) (*papi.StoryListExtraResponse, error) {
	suid := viber.GetSuid(ctx).String()
	infos, err := opuser.FindAllUserExtraStoryPartInfos(ctx, suid)
	if err != nil {
		return nil, err
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Id < infos[j].Id })
	for _, info := range infos {
		sort.Slice(info.Stories, func(i, j int) bool { return info.Stories[i].Id < info.Stories[j].Id })
		for _, story := range info.Stories {
			sort.Slice(story.Episodes, func(i, j int) bool {
				return story.Episodes[i].Episode < story.Episodes[j].Episode
			})
		}
	}
	return &papi.StoryListExtraResponse{
		Infos:          infos,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *StoryServer) ReadEvent(ctx context.Context, req *papi.StoryReadEventRequest) (*papi.StoryReadEventResponse, error) {
	if req.GetEventStoryId() == "" || req.GetEpisode() <= 0 {
		return nil, viber.InvalidArgumentf("eventStoryId and a positive episode are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.StoryReadEventResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		master, err := opmaster.FindEventStory(ctx, req.EventStoryId)
		if err != nil {
			return nil, err
		}
		storyId := ""
		for _, episode := range master.Episodes {
			if episode.Episode == req.Episode {
				storyId = episode.StoryId
				break
			}
		}
		if storyId == "" {
			return nil, viber.InvalidArgumentf("event story episode does not exist")
		}
		info, err := opuser.FindUserEventStoryInfo(ctx, suid, req.EventStoryId)
		if err != nil {
			return nil, err
		}
		if !eventEpisodeUnlocked(info, req.Episode) {
			return nil, status.Error(codes.PermissionDenied, "event story episode is locked")
		}
		rewards, updated, err := readUserStory(ctx, suid, storyId, true)
		if err != nil {
			return nil, err
		}
		response = &papi.StoryReadEventResponse{Rewards: rewards, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *StoryServer) ReadExtra(ctx context.Context, req *papi.StoryReadExtraRequest) (*papi.StoryReadExtraResponse, error) {
	if req.GetExtraStoryId() == "" || req.GetEpisode() <= 0 {
		return nil, viber.InvalidArgumentf("extraStoryId and a positive episode are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.StoryReadExtraResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		master, err := opmaster.FindExtraStory(ctx, req.ExtraStoryId)
		if err != nil {
			return nil, err
		}
		storyId := ""
		for _, episode := range master.Episodes {
			if episode.Episode == req.Episode {
				storyId = episode.StoryId
				break
			}
		}
		if storyId == "" {
			return nil, viber.InvalidArgumentf("extra story episode does not exist")
		}
		part, err := opuser.FindUserExtraStoryPartInfo(ctx, suid, master.ExtraStoryPartId)
		if err != nil {
			return nil, err
		}
		if !extraEpisodeUnlocked(part, req.ExtraStoryId, req.Episode) {
			return nil, status.Error(codes.PermissionDenied, "extra story episode is locked")
		}
		rewards, updated, err := readUserStory(ctx, suid, storyId, true)
		if err != nil {
			return nil, err
		}
		response = &papi.StoryReadExtraResponse{Rewards: rewards, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *StoryServer) ReadCharacterCompanyEnjoyStory(
	ctx context.Context,
	req *papi.StoryReadCharacterCompanyEnjoyStoryRequest,
) (*papi.StoryReadCharacterCompanyEnjoyStoryResponse, error) {
	if req.GetCharacterId() == "" || req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("characterId and a positive number are required")
	}
	masterCharacter, err := opmaster.FindCharacter(ctx, req.CharacterId)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	storyId := ""
	for _, definition := range masterCharacter.GetCompanyEnjoyStories() {
		if definition.GetNumber() == req.GetNumber() {
			storyId = definition.GetStoryId()
			break
		}
	}
	if storyId == "" {
		return nil, viber.InvalidArgumentf("character company enjoy story does not exist")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.StoryReadCharacterCompanyEnjoyStoryResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		rewards, updated, err := readUserStory(ctx, suid, storyId, false)
		if err != nil {
			return nil, err
		}
		response = &papi.StoryReadCharacterCompanyEnjoyStoryResponse{
			Rewards: rewards, CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func readUserStory(
	ctx context.Context,
	suid string,
	storyId string,
	allowCreate bool,
) ([]*ptransaction.RewardResult, *papi.UpdatedData, error) {
	master, err := opmaster.FindStory(ctx, storyId)
	if err != nil {
		return nil, nil, err
	}
	story, err := opuser.FindUserStory(ctx, suid, storyId)
	if err != nil {
		if !allowCreate || !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil, err
		}
		story = &ptransaction.UserStory{StoryId: storyId, StatusType: penum.StoryStatusType_StoryStatusType_Unread}
	}
	if story.IsInvalid {
		return nil, nil, status.Error(codes.PermissionDenied, "story is unavailable")
	}
	updated := &papi.UpdatedData{}
	if story.StatusType == penum.StoryStatusType_StoryStatusType_Read {
		return []*ptransaction.RewardResult{}, updated, nil
	}
	if story.StatusType != penum.StoryStatusType_StoryStatusType_Unread {
		return nil, nil, status.Error(codes.PermissionDenied, "story is locked")
	}
	story.StatusType = penum.StoryStatusType_StoryStatusType_Read
	if err := opuser.UpsertUserStory(ctx, suid, story); err != nil {
		return nil, nil, err
	}
	updated.Stories = append(updated.Stories, story)
	rewards, err := grantStoryReward(ctx, suid, master, updated)
	if err != nil {
		return nil, nil, err
	}
	if err := viber.ReconcileNarrativeSchedulesAfterStory(ctx, suid, time.Now().UnixMilli(), storyId, updated); err != nil {
		return nil, nil, err
	}
	return rewards, updated, nil
}

func grantStoryReward(
	ctx context.Context,
	suid string,
	story *pmaster.Story,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if story.RewardId == "" {
		return []*ptransaction.RewardResult{}, nil
	}
	resources, err := viber.ParseAutoReward(story.RewardId)
	if err != nil {
		return nil, err
	}
	if resources == nil {
		reward, err := viber.FindMasterReward(ctx, story.RewardId)
		if err != nil {
			return nil, err
		}
		resources = reward.Resources
	}
	results := make([]*ptransaction.RewardResult, 0, len(resources))
	for _, resource := range resources {
		if resource.Amount <= 0 {
			return nil, fmt.Errorf("story %q has a non-positive reward", story.Id)
		}
		granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, granted...)
	}
	return results, nil
}

func eventEpisodeUnlocked(info *papi.EventStoryInfo, episode int32) bool {
	for _, value := range info.Episodes {
		if value.Episode == episode {
			return value.Unlocked
		}
	}
	return false
}

func extraEpisodeUnlocked(info *papi.ExtraStoryPartInfo, storyId string, episode int32) bool {
	if !info.Unlocked {
		return false
	}
	for _, story := range info.Stories {
		if story.Id != storyId || !story.Unlocked {
			continue
		}
		for _, value := range story.Episodes {
			if value.Episode == episode {
				return value.Unlocked
			}
		}
	}
	return false
}
