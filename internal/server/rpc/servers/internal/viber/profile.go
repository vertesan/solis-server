package viber

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

// LoadSelfProfileInfo projects the caller's current business card for Profile
// and Hierarchy. Referenced cards and photos must belong to the same user.
func LoadSelfProfileInfo(ctx context.Context, suid string) (*papi.ProfileInfo, error) {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, err
	}
	profile, err := opuser.FindUserProfile(ctx, suid)
	if err != nil {
		return nil, err
	}
	info := &papi.ProfileInfo{
		UserId:               public.GetPublicUserId(),
		Name:                 user.GetName(),
		ManagerExp:           user.GetManagerExp(),
		ManagerLevel:         user.GetManagerLevel(),
		EmblemId:             user.GetEmblemId(),
		FavoriteCharacterIds: append([]string(nil), profile.GetFavoriteCharacterIds()...),
		BackgroundType:       profile.GetBackgroundType(),
		LayoutType:           profile.GetLayoutType(),
		LastLoginTime:        user.GetLastLoginTime(),
		DecorationId:         profile.GetDecorationId(),
		InformationType:      profile.GetInformationType(),
		ColorType:            profile.GetColorType(),
	}
	switch profile.GetBackgroundType() {
	case penum.ProfileBackgroundType_ProfileBackgroundType_Card:
		card, err := opuser.FindUserCard(ctx, suid, profile.GetFavoriteCardId())
		if err != nil {
			return nil, err
		}
		info.FavoriteCard = &papi.FavoriteCardInfo{
			CardId:         card.GetCardId(),
			Rarity:         card.GetRarity(),
			DisplayType:    profile.GetFavoriteCardDisplayType(),
			EvolutionLevel: card.GetEvolutionLevel(),
		}
	case penum.ProfileBackgroundType_ProfileBackgroundType_Photo:
		photo, err := opuser.FindUserPhoto(ctx, suid, profile.GetFavoritePhotoId())
		if err != nil {
			return nil, err
		}
		info.FavoritePhoto = &papi.FavoritePhotoInfo{
			PhotoId:   photo.GetPhotoId(),
			AssetId:   photo.GetAssetId(),
			ImageType: photo.GetImageType(),
		}
	}
	if user.GetBuddyCardId() != "" {
		card, err := opuser.FindUserCard(ctx, suid, user.GetBuddyCardId())
		if err != nil {
			return nil, err
		}
		info.BuddyCardInfo = &papi.BuddyCardInfo{
			CardId:         card.GetCardId(),
			Rarity:         card.GetRarity(),
			Level:          card.GetLevel(),
			EvolutionLevel: card.GetEvolutionLevel(),
		}
	}
	return info, nil
}
