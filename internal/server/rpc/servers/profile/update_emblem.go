package profile

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) UpdateEmblem(ctx context.Context, req *papi.ProfileUpdateEmblemRequest) (*papi.ProfileUpdateEmblemResponse, error) {
	if req == nil {
		return nil, viber.InvalidArgumentf("request is required")
	}
	common, err := updateProfileUser(ctx, func(ctx context.Context, suid string, user *ptransaction.User) error {
		if req.GetEmblemId() != "" {
			if _, err := opuser.FindUserEmblem(ctx, suid, req.GetEmblemId()); err != nil {
				return err
			}
		}
		user.EmblemId = req.GetEmblemId()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &papi.ProfileUpdateEmblemResponse{CommonResponse: common}, nil
}
