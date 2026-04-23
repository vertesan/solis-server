package consts

import "vertesan/solis-server/proto/papi"

var UserInitialStaticExtraStoryPartInfos = []*papi.ExtraStoryPartInfo{
	{
		Id:       "ex-st-part-limited-13",
		Unlocked: true,
		Stories:  []*papi.ExtraStoryInfo{},
	},
}

var UserInitialDynamicExtraStoryPartIds = []string{
	"ex-story-part-birthday-01",
	"ex-story-part-special-01",
}
