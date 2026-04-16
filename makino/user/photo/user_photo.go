package photo

import (
	"context"
	"fmt"
	"time"
	"vertesan/solis-server/db/crud/opuser"
	"vertesan/solis-server/proto/penum"
	"vertesan/solis-server/proto/ptransaction"

	"github.com/google/uuid"
)

func UpsertInitialManaPhoto(ctx context.Context, suid string) error {
	if err := opuser.UpsertUserPhoto(ctx, suid, &ptransaction.UserPhoto{
		PhotoId:   uuid.NewString(),
			AssetId:   "phot-login-5-0001",
			ImageType: penum.PhotoImageType_PhotoImageType_Asset,
			Name:      "長瀬麻奈",
			Rarity:    5,
			PlaceName: "First Step",
			EventName: "メモリアルフォト",
			Level:     10,
			Abilities: []*ptransaction.UserPhotoAbility{
				{
					PhotoAbilityId: "pab-vocal_multiply_distribution-5",
					EffectValue:    150,
					Grade:          1,
					IsAvailable:    true,
					MissionId:      "",
				},
				{
					PhotoAbilityId: "pab-dance_multiply_distribution-5",
					EffectValue:    150,
					Grade:          1,
					IsAvailable:    true,
					MissionId:      "",
				},
				{
					PhotoAbilityId: "pab-visual_multiply_distribution-5",
					EffectValue:    150,
					Grade:          1,
					IsAvailable:    true,
					MissionId:      "",
				},
				{
					PhotoAbilityId: "pab-technique_add-5",
					EffectValue:    26,
					Grade:          7,
					IsAvailable:    true,
					MissionId:      "",
				},
				{
					PhotoAbilityId: "pab-passive-skill_sk-phot-release-login-5-01",
					EffectValue:    1,
					Grade:          1,
					IsAvailable:    true,
					MissionId:      "",
				},
			},
			ShootingTime:               time.Now().UnixMilli(),
			FocusCharacterId:           "char-mna",
			CharacterIds:               []string{},
			Locked:                     false,
			Rerollable:                 false,
			PhotoRecipeId:              "",
			IsDreamBase:                false,
			DreamMaterialPhotoRarity:   0,
			DreamMaterialPhotoLevel:    0,
			DreamMaterialPhotoRecipeId: "",
	}); err != nil {
		return fmt.Errorf("failed upserting initialManaPhoto: %w", err)
	}
	return nil
}
