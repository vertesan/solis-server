package love

import (
	"context"
	"slices"
	"unicode/utf8"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *LoveServer) UpdateSaveName(ctx context.Context, req *papi.LoveUpdateSaveNameRequest) (*papi.LoveUpdateSaveNameResponse, error) {
	if !utf8.ValidString(req.GetName()) || utf8.RuneCountInString(req.GetName()) > 20 {
		return nil, viber.InvalidArgumentf("save name must be at most 20 characters")
	}
	suid := viber.GetSuid(ctx).String()
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		_, state, love, err := loadLove(ctx, suid, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		if err := validateLoveSave(ctx, love, req.GetSlotNumber()); err != nil {
			return nil, err
		}
		index := slices.IndexFunc(state.Saves, func(save umodel.UserLoveSave) bool { return save.Info.GetSlotNumber() == req.GetSlotNumber() })
		if index < 0 {
			return nil, status.Error(codes.NotFound, "save slot is empty")
		}
		state.Saves[index].Info.Name = req.GetName()
		return nil, opuser.UpsertUserLoveState(ctx, state)
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.LoveUpdateSaveNameResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
