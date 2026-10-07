package exercise

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ExerciseServer) Top(ctx context.Context, _ *papi.Empty) (*papi.ExerciseTopResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var exercises []*papi.ExerciseInfo
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		rows, err := opmaster.FindAllExercises(ctx)
		if err != nil {
			return nil, err
		}
		saved, err := opuser.FindAllUserExerciseDecks(ctx, suid)
		if err != nil {
			return nil, err
		}
		positions := make(map[string][]*papi.ExerciseDeckPosition, len(saved))
		for _, deck := range saved {
			positions[deck.ExerciseID] = deck.Data.GetPositions()
		}
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
		// The client presents individual lesson conditions from the master. The
		// area-wide requirement is also checked here and by all mutating RPCs.
		area, err := opmaster.FindArea(ctx, "area-exercise")
		if err != nil {
			return nil, err
		}
		available := true
		for _, condition := range []string{area.GetViewConditionId(), area.GetUnlockConditionId()} {
			ok, err := evaluator.Satisfied(condition)
			if err != nil {
				return nil, err
			}
			available = available && ok
		}
		exercises = make([]*papi.ExerciseInfo, 0, len(rows))
		for _, row := range rows {
			unlocked, err := evaluator.Satisfied(row.GetUnlockConditionId())
			if err != nil {
				return nil, err
			}
			exercises = append(exercises, &papi.ExerciseInfo{ExerciseId: row.GetId(), Unlocked: available && unlocked, Positions: positions[row.GetId()]})
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ExerciseTopResponse{Exercises: exercises, CommonResponse: viber.NewCommonResponse(nil)}, nil
}
