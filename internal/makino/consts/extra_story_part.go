package consts

import "github.com/vertesan/solis-server/internal/proto/papi"

func GetUserInitialStaticExtraStoryPartInfos() []*papi.ExtraStoryPartInfo {
	return []*papi.ExtraStoryPartInfo{
		{
			Id:       "ex-st-part-limited-13",
			Unlocked: true,
			Stories:  []*papi.ExtraStoryInfo{},
		},
	}
}

func GetUserInitialDynamicExtraStoryPartIds() []string {
	return []string{
		"ex-story-part-birthday-01",
		"ex-story-part-special-01",
	}
}
