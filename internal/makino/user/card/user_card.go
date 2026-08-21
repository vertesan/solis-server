package card

import (
	"context"
	"fmt"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func SpawnUserCardById(ctx context.Context, cardId string) (*ptransaction.UserCard, error) {
	card, err := opmaster.FindCard(ctx, cardId)
	if err != nil {
		return nil, err
	}
	return SpawnUserCard(ctx, card)
}

func SpawnUserCard(ctx context.Context, card *pmaster.Card) (*ptransaction.UserCard, error) {
	settingId, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingId)
	if err != nil {
		return nil, err
	}
	cardRarity, err := opmaster.FindCardRarity(ctx, int64(card.InitialRarity))
	if err != nil {
		return nil, err
	}
	evolutionLevel, evolutionExp, err := InitialEvolutionState(ctx, card)
	if err != nil {
		return nil, err
	}
	params, err := CalcUserCardBaseAttr(ctx, card, 1, card.InitialRarity)
	if err != nil {
		return nil, fmt.Errorf("failed to CalcUserCardBaseAttr: %w", err)
	}
	photoCount, skillIds, abilityIds, err := CalcUnlocksWithEvolution(ctx, 1, evolutionLevel, card)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	userCard := &ptransaction.UserCard{
		CardId:                   card.Id,
		TotalExp:                 0,
		RarityTotalExp:           cardRarity.RequiredExp,
		SkillLevel1:              1,
		SkillLevel2:              1,
		SkillLevel3:              1,
		SkillLevel4:              1,
		EvolutionLevelTotalExp:   evolutionExp,
		LiveAbilityLevel:         1,
		ActivityAbilityLevel:     1,
		ObtainedTime:             now,
		Supported:                false,
		LastEnhanceTime:          0,
		RankTotalExp:             0,
		Level:                    1,
		Rarity:                   card.InitialRarity,
		Vocal:                    params.Vocal,
		Dance:                    params.Dance,
		Visual:                   params.Visual,
		Stamina:                  params.Stamina,
		Mental:                   int64(setting.CardDefaultMental),
		Technique:                int64(setting.CardDefaultTechnique),
		SkillId1:                 skillIds[0],
		SkillId2:                 skillIds[1],
		SkillId3:                 skillIds[2],
		SkillId4:                 skillIds[3],
		LiveAbilityId:            abilityIds[0],
		ActivityAbilityId:        abilityIds[1],
		PhotoEquipableCount:      photoCount,
		BaseLevel:                1,
		BaseVocal:                params.Vocal,
		BaseDance:                params.Dance,
		BaseVisual:               params.Visual,
		BaseStamina:              params.Stamina,
		BaseSkillLevel1:          1,
		BaseSkillLevel2:          1,
		BaseSkillLevel3:          1,
		BaseSkillLevel4:          1,
		BaseSkillId1:             skillIds[0],
		BaseSkillId2:             skillIds[1],
		BaseSkillId3:             skillIds[2],
		BaseSkillId4:             skillIds[3],
		BaseLiveAbilityLevel:     1,
		BaseActivityAbilityLevel: 1,
		BaseLiveAbilityId:        abilityIds[0],
		BaseActivityAbilityId:    abilityIds[1],
		BasePhotoEquipableCount:  photoCount,
		Rank:                     0,
		EvolutionLevel:           evolutionLevel,
	}
	return userCard, nil
}
