package exercise

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	training "github.com/vertesan/solis-server/internal/makino/exercise"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func loadAvailableCatalog(ctx context.Context, suid, id string) (*training.Catalog, error) {
	if id == "" {
		return nil, viber.InvalidArgumentf("exerciseId is required")
	}
	catalog, err := training.LoadCatalog(ctx, id)
	if err != nil {
		return nil, err
	}
	area, err := opmaster.FindArea(ctx, catalog.Quest.GetAreaId())
	if err != nil {
		return nil, err
	}
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
	for _, condition := range []string{area.GetViewConditionId(), area.GetUnlockConditionId(), catalog.Exercise.GetUnlockConditionId()} {
		unlocked, err := evaluator.Satisfied(condition)
		if err != nil {
			return nil, err
		}
		if !unlocked {
			return nil, viber.FailedPreconditionf("exercise is locked")
		}
	}
	return catalog, nil
}

func loadSavedPositions(ctx context.Context, suid, id string) ([]*papi.ExerciseDeckPosition, error) {
	deck, err := opuser.FindUserExerciseDeck(ctx, suid, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return deck.Data.GetPositions(), nil
}
