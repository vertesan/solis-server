package photo

import (
	"context"
	"fmt"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"github.com/google/uuid"
)

// NewUserPhotoFromAllInOne materializes an independently-owned user photo from
// its master-data definition. A fresh ID is required because a player can own
// more than one photo backed by the same asset.
func NewUserPhotoFromAllInOne(master *pmaster.PhotoAllInOne) *ptransaction.UserPhoto {
	abilities := make([]*ptransaction.UserPhotoAbility, 0, len(master.GetAbilities()))
	for _, ability := range master.GetAbilities() {
		if ability == nil {
			continue
		}
		abilities = append(abilities, &ptransaction.UserPhotoAbility{
			PhotoAbilityId: ability.GetPhotoAbilityId(),
			EffectValue:    int64(ability.GetEffectValue()),
			MissionId:      ability.GetMissionId(),
			Grade:          ability.GetGrade(),
			IsAvailable:    true,
		})
	}

	return &ptransaction.UserPhoto{
		PhotoId:          uuid.NewString(),
		AssetId:          master.GetAssetId(),
		ImageType:        penum.PhotoImageType_PhotoImageType_Asset,
		CharacterIds:     append([]string(nil), master.GetCharacterIds()...),
		Name:             master.GetName(),
		Rarity:           master.GetRarity(),
		PlaceName:        master.GetPlaceName(),
		EventName:        master.GetEventName(),
		Level:            master.GetLevel(),
		Abilities:        abilities,
		ShootingTime:     master.GetShootingTime(),
		FocusCharacterId: master.GetFocusCharacterId(),
	}
}

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
