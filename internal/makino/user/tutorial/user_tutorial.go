package tutorial

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

func SpawnUserInitialTutorial(ctx context.Context) ([]*ptransaction.UserTutorial, error) {
	tutorials, err := opmaster.FindAllTutorials(ctx)
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
