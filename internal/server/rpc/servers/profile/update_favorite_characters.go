package profile

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) UpdateFavoriteCharacters(ctx context.Context, req *papi.ProfileUpdateFavoriteCharactersRequest) (*papi.ProfileUpdateFavoriteCharactersResponse, error) {
	if req == nil || len(req.GetFavoriteCharacterIds()) > 3 {
		return nil, viber.InvalidArgumentf("at most three favorite characters may be selected")
	}
	ids := req.GetFavoriteCharacterIds()
	for i, id := range ids {
		if id == "" || slices.Contains(ids[:i], id) {
			return nil, viber.InvalidArgumentf("favorite character IDs must be nonempty and distinct")
		}
	}
	common, err := updateProfile(ctx, func(ctx context.Context, suid string, profile *ptransaction.UserProfile) error {
		for _, id := range ids {
			if _, err := opuser.FindUserCharacter(ctx, suid, id); err != nil {
				return err
			}
		}
		profile.FavoriteCharacterIds = slices.Clone(ids)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &papi.ProfileUpdateFavoriteCharactersResponse{CommonResponse: common}, nil
}
