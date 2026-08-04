package card

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"time"
)

func SpawnUserCardById(cardId string) (*ptransaction.UserCard, error) {
	card, err := opmaster.FindCard(context.Background(), cardId)
	if err != nil {
		return nil, err
	}
	return SpawnUserCard(card)
}

func SpawnUserCard(card *pmaster.Card) (*ptransaction.UserCard, error) {
	setting, err := opmaster.FindSetting(context.Background(), config.GetGameSettingId())
	if err != nil {
		return nil, err
	}
	cardRarity, err := opmaster.FindCardRarity(context.Background(), int64(card.InitialRarity))
	if err != nil {
		return nil, err
	}
	params, err := CalcUserCardBaseAttr(card, 1, card.InitialRarity)
	if err != nil {
		return nil, fmt.Errorf("failed to CalcUserCardBaseAttr: %w", err)
	}
	displayType, err := CalcCardDisplayType(card.Id, card.InitialRarity, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to CalcCardDisplayType: %w", err)
	}
	photoCount, skillIds, abilityIds, err := CalcUnlocks(1, card)
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
		LiveAbilityLevel:         1,
		ActivityAbilityLevel:     1,
		ObtainedTime:             now,
		Supported:                false,
		LastEnhanceTime:          0,
		RankTotalExp:             0,
		DisplayType:              displayType,
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
		SkillId4:                 card.SkillId4,
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
		BaseSkillId4:             card.SkillId4,
		BaseLiveAbilityLevel:     1,
		BaseActivityAbilityLevel: 1,
		BaseLiveAbilityId:        abilityIds[0],
		BaseActivityAbilityId:    abilityIds[1],
		BasePhotoEquipableCount:  photoCount,
		Rank:                     0,
		EvolutionLevel:           0,
	}
	return userCard, nil
}
