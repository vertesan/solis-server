package extrastory

import (
	"context"
	"fmt"
	"vertesan/solis-server/db/crud/opmaster"
	"vertesan/solis-server/makino/consts"
	"vertesan/solis-server/proto/papi"
)

func SpawnUserInitialExtraStoryPartInfos(ctx context.Context) ([]*papi.ExtraStoryPartInfo, error) {
	parts, err := opmaster.FindExtraStoryPartsByIds(ctx, consts.UserInitialDynamicExtraStoryPartIds)
	if err != nil {
		return nil, fmt.Errorf("failed to find extra story parts: %w", err)
	}

	storyIds := make([]string, 0)
	for _, part := range parts {
		storyIds = append(storyIds, part.ExtraStoryIds...)
	}
	stories, err := opmaster.FindExtraStoriesByIds(ctx, storyIds)
	if err != nil {
		return nil, fmt.Errorf("failed to find extra stories: %w", err)
	}
	storyById := make(map[string]*papi.ExtraStoryInfo, len(stories))
	for _, s := range stories {
		episodes := make([]*papi.ExtraStoryEpisodeInfo, 0, len(s.Episodes))
		for _, ep := range s.Episodes {
			episodes = append(episodes, &papi.ExtraStoryEpisodeInfo{
				Episode:  ep.Episode,
				Unlocked: true,
			})
		}
		storyById[s.Id] = &papi.ExtraStoryInfo{
			Id:       s.Id,
			Unlocked: true,
			Episodes: episodes,
		}
	}

	infos := make([]*papi.ExtraStoryPartInfo, 0, len(consts.UserInitialStaticExtraStoryPartInfos)+len(parts))
	infos = append(infos, consts.UserInitialStaticExtraStoryPartInfos...)
	for _, part := range parts {
		partStories := make([]*papi.ExtraStoryInfo, 0, len(part.ExtraStoryIds))
		for _, sid := range part.ExtraStoryIds {
			if s, ok := storyById[sid]; ok {
				partStories = append(partStories, s)
			}
		}
		infos = append(infos, &papi.ExtraStoryPartInfo{
			Id:       part.Id,
			Unlocked: true,
			Stories:  partStories,
		})
	}
	return infos, nil
}
