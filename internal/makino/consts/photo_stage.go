package consts

import "github.com/vertesan/solis-server/internal/proto/papi"

func GetUserInitialPhotoStages() []*papi.PhotoStage {
	return []*papi.PhotoStage{
		{StageId: "stage-live-house-00-00", IsUnlocked: true},
		{StageId: "stage-live-house-01-00", IsUnlocked: true},
		{StageId: "stage-live-unique-00-00", IsUnlocked: true},
		{StageId: "stage-live-unique-01-00", IsUnlocked: true},
		{StageId: "stage-live-unique-02-00", IsUnlocked: true},
		{StageId: "stage-live-unique-03-00", IsUnlocked: true},
	}
}
