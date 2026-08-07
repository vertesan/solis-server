package character

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"time"
)

func IsPlayableCharacter(character *pmaster.Character) bool {
	return character.Type != penum.CharacterType_CharacterType_Npc
}

func SpawnUserCharacterById(ctx context.Context, characterId string) (*ptransaction.UserCharacter, error) {
	character, err := opmaster.FindCharacter(ctx, characterId)
	if err != nil {
		return nil, err
	}
	return SpawnUserCharacter(ctx, character)
}

func SpawnUserCharacter(ctx context.Context, character *pmaster.Character) (*ptransaction.UserCharacter, error) {
	activityLevel, err := opmaster.FindCharacterActivityLevel(ctx, 1)
	if err != nil {
		return nil, err
	}
	companyBonusParameterInfos := []*ptransaction.CompanyBonusParameterInfo{
		{BonusParameterType: penum.ParameterType_ParameterType_Dance, BonusParameterUpPermil: 0},
		{BonusParameterType: penum.ParameterType_ParameterType_Vocal, BonusParameterUpPermil: 0},
		{BonusParameterType: penum.ParameterType_ParameterType_Visual, BonusParameterUpPermil: 0},
		{BonusParameterType: penum.ParameterType_ParameterType_Stamina, BonusParameterUpPermil: 0},
		{BonusParameterType: penum.ParameterType_ParameterType_Mental, BonusParameterUpPermil: 0},
		{BonusParameterType: penum.ParameterType_ParameterType_Technique, BonusParameterUpPermil: 0},
	}
	now := time.Now().UnixMilli()
	userCharacter := &ptransaction.UserCharacter{
		CharacterId:                character.Id,
		ActivityExp:                0,
		ActivityStamina:            activityLevel.MaxStamina,
		LightFanAmount:             0,
		MiddleFanAmount:            0,
		HeavyFanAmount:             0,
		FavoriteCostumeId:          character.DefaultCostumeId,
		StaminaUpdatedTime:         now,
		InActivity:                 false,
		Reliability:                0,
		LiveHighestScore:           0,
		CharacterOnlyPhotoAmount:   0,
		LiveCostumeId:              character.DefaultLiveCostumeId,
		FavoriteHairId:             "",
		LiveHairId:                 "",
		CompanyEnjoyPoint:          0,
		LastHomeTalkFirstReadTime:  0,
		ActivityLevel:              1,
		AudienceCandidateAmount:    10,
		CompanyBonusParameterInfos: companyBonusParameterInfos,
	}
	return userCharacter, nil
}
