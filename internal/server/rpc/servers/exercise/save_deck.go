package exercise

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/protobuf/proto"
)

func (s *ExerciseServer) SaveDeck(ctx context.Context, req *papi.ExerciseSaveDeckRequest) (*papi.ExerciseSaveDeckResponse, error) {
	suid := viber.GetSuid(ctx).String()
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, err := loadAvailableCatalog(ctx, suid, req.GetExerciseId())
		if err != nil {
			return nil, err
		}
		if _, err = catalog.ResolvePositions(req.GetPositions(), false); err != nil {
			return nil, err
		}
		positions := make([]*papi.ExerciseDeckPosition, 0, len(req.GetPositions()))
		for _, position := range req.GetPositions() {
			positions = append(positions, proto.Clone(position).(*papi.ExerciseDeckPosition))
		}
		sort.Slice(positions, func(i, j int) bool { return positions[i].GetPosition() < positions[j].GetPosition() })
		return nil, opuser.UpsertUserExerciseDeck(ctx, &umodel.UserExerciseDeck{
			SUID: suid, ExerciseID: req.GetExerciseId(), Data: &papi.ExerciseInfo{ExerciseId: req.GetExerciseId(), Positions: positions},
		})
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ExerciseSaveDeckResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
