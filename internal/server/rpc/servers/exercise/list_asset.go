package exercise

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/transaction"
	training "github.com/vertesan/solis-server/internal/makino/exercise"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ExerciseServer) ListAsset(ctx context.Context, req *papi.ExerciseListAssetRequest) (*papi.ExerciseListAssetResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var assets []*papi.LiveCharacterAssetInfo
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, err := loadAvailableCatalog(ctx, suid, req.GetExerciseId())
		if err != nil {
			return nil, err
		}
		saved, err := loadSavedPositions(ctx, suid, req.GetExerciseId())
		if err != nil {
			return nil, err
		}
		positions, err := catalog.ResolvePositions(saved, true)
		if err != nil {
			return nil, err
		}
		assets, err = training.CharacterAssets(ctx, catalog, positions)
		return nil, err
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ExerciseListAssetResponse{CharacterAssets: assets, CommonResponse: viber.NewCommonResponse(nil)}, nil
}
