package gacha

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *GachaServer) SelectCard(ctx context.Context, req *papi.GachaSelectCardRequest) (*papi.GachaSelectCardResponse, error) {
	if req.GetGachaId() == "" {
		return nil, status.Error(codes.InvalidArgument, "gachaId is required")
	}
	suid := viber.GetSuid(ctx).String()
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, err := viber.LoadGachaMasterCatalog(ctx)
		if err != nil {
			return nil, err
		}
		masterGacha := catalog.Gachas[req.GetGachaId()]
		if masterGacha == nil {
			return nil, status.Error(codes.NotFound, "gacha not found")
		}
		if int32(len(req.GetSelectedCardIds())) != masterGacha.GetSelectPickupAmount() {
			return nil, status.Errorf(codes.InvalidArgument, "exactly %d cards must be selected", masterGacha.GetSelectPickupAmount())
		}
		allowed := viber.StringSet(masterGacha.GetSelectCardIds())
		seen := make(map[string]struct{}, len(req.GetSelectedCardIds()))
		for _, cardID := range req.GetSelectedCardIds() {
			if _, ok := allowed[cardID]; !ok {
				return nil, status.Errorf(codes.InvalidArgument, "card %q cannot be selected", cardID)
			}
			if _, ok := seen[cardID]; ok {
				return nil, status.Errorf(codes.InvalidArgument, "card %q is selected more than once", cardID)
			}
			seen[cardID] = struct{}{}
		}
		now := time.Now().UnixMilli()
		state, err := currentGachaInfo(ctx, suid, catalog, masterGacha, now)
		if err != nil {
			return nil, err
		}
		if !state.GetUnlocked() {
			return nil, status.Error(codes.PermissionDenied, "gacha is locked")
		}
		state.SelectedCardIds = append([]string(nil), req.GetSelectedCardIds()...)
		return nil, opuser.UpsertUserGachaInfo(ctx, suid, state)
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.GachaSelectCardResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
