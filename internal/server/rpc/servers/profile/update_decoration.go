package profile

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) UpdateDecoration(ctx context.Context, req *papi.ProfileUpdateDecorationRequest) (*papi.ProfileUpdateDecorationResponse, error) {
	if req == nil {
		return nil, viber.InvalidArgumentf("request is required")
	}
	common, err := updateProfile(ctx, func(ctx context.Context, suid string, profile *ptransaction.UserProfile) error {
		if req.GetDecorationId() != "" {
			if _, err := opuser.FindUserDecoration(ctx, suid, req.GetDecorationId()); err != nil {
				return err
			}
		}
		profile.DecorationId = req.GetDecorationId()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &papi.ProfileUpdateDecorationResponse{CommonResponse: common}, nil
}
