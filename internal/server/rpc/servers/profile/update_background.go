package profile

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) UpdateBackground(ctx context.Context, req *papi.ProfileUpdateBackgroundRequest) (*papi.ProfileUpdateBackgroundResponse, error) {
	if req.GetBackgroundType() < penum.ProfileBackgroundType_ProfileBackgroundType_Default || req.GetBackgroundType() > penum.ProfileBackgroundType_ProfileBackgroundType_Photo {
		return nil, viber.InvalidArgumentf("a valid backgroundType is required")
	}
	if _, ok := penum.CardDisplayType_name[int32(req.GetFavoriteCardDisplayType())]; !ok {
		return nil, viber.InvalidArgumentf("favoriteCardDisplayType is invalid")
	}
	common, err := updateProfile(ctx, func(ctx context.Context, suid string, profile *ptransaction.UserProfile) error {
		switch req.GetBackgroundType() {
		case penum.ProfileBackgroundType_ProfileBackgroundType_Card:
			if req.GetFavoriteCardId() == "" || req.GetFavoritePhotoId() != "" {
				return viber.InvalidArgumentf("card background requires only favoriteCardId")
			}
			card, err := opuser.FindUserCard(ctx, suid, req.GetFavoriteCardId())
			if err != nil {
				return err
			}
			// Zero selects the card's default artwork in the captured client.
			if req.GetFavoriteCardDisplayType() != penum.CardDisplayType_CardDisplayType_Unknown {
				if err := viber.ValidateCardDisplayType(ctx, card, req.GetFavoriteCardDisplayType()); err != nil {
					return err
				}
			}
		case penum.ProfileBackgroundType_ProfileBackgroundType_Photo:
			if req.GetFavoritePhotoId() == "" || req.GetFavoriteCardId() != "" || req.GetFavoriteCardDisplayType() != penum.CardDisplayType_CardDisplayType_Unknown {
				return viber.InvalidArgumentf("photo background requires only favoritePhotoId")
			}
			if _, err := opuser.FindUserPhoto(ctx, suid, req.GetFavoritePhotoId()); err != nil {
				return err
			}
		default:
			if req.GetFavoriteCardId() != "" || req.GetFavoritePhotoId() != "" || req.GetFavoriteCardDisplayType() != penum.CardDisplayType_CardDisplayType_Unknown {
				return viber.InvalidArgumentf("default background does not accept a card or photo")
			}
		}
		profile.BackgroundType = req.GetBackgroundType()
		profile.FavoriteCardId = req.GetFavoriteCardId()
		profile.FavoritePhotoId = req.GetFavoritePhotoId()
		profile.FavoriteCardDisplayType = req.GetFavoriteCardDisplayType()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &papi.ProfileUpdateBackgroundResponse{CommonResponse: common}, nil
}
