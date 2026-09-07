package profile

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) UpdateBuddyCard(ctx context.Context, req *papi.ProfileUpdateBuddyCardRequest) (*papi.ProfileUpdateBuddyCardResponse, error) {
	if req == nil {
		return nil, viber.InvalidArgumentf("request is required")
	}
	common, err := updateProfileUser(ctx, func(ctx context.Context, suid string, user *ptransaction.User) error {
		if req.GetCardId() != "" {
			if _, err := opuser.FindUserCard(ctx, suid, req.GetCardId()); err != nil {
				return err
			}
		}
		user.BuddyCardId = req.GetCardId()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &papi.ProfileUpdateBuddyCardResponse{CommonResponse: common}, nil
}
