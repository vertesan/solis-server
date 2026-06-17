package tutorial

import (
	"context"
	"vertesan/solis-server/db/store/opmaster"
	"vertesan/solis-server/proto/ptransaction"
)

func SpawnUserInitialTutorial() ([]*ptransaction.UserTutorial, error) {
	tutorials, err := opmaster.FindAllTutorials(context.Background())
	if err != nil {
		return nil, err
	}
	userTutorials := make([]*ptransaction.UserTutorial, 0, len(tutorials))
	for _, t := range tutorials {
		var maxStep int32
		for _, s := range t.StepInfo {
			if s.Step > maxStep {
				maxStep = s.Step
			}
		}
		userTutorials = append(userTutorials, &ptransaction.UserTutorial{
			TutorialType: t.Type,
			Step:         int64(maxStep),
		})
	}
	return userTutorials, nil
}
