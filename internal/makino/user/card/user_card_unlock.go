package card

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

// returned value: photoCount, skillIds, abilityIds
func CalcUnlocks(ctx context.Context, lteLevel int32, card *pmaster.Card) (int32, []string, []string, error) {
	releaseMap, err := CalcReleaseCounts(ctx, lteLevel, card)
	if err != nil {
		return 0, nil, nil, err
	}
	photoCount := releaseMap[penum.CardLevelReleaseType_CardLevelReleaseType_Photo]
	skillIds := convertSkillIds(
		releaseMap[penum.CardLevelReleaseType_CardLevelReleaseType_Skill],
		card,
	)
	abilityIds := convertAbilities(
		releaseMap[penum.CardLevelReleaseType_CardLevelReleaseType_LiveAbility],
		releaseMap[penum.CardLevelReleaseType_CardLevelReleaseType_ActivityAbility],
		card,
	)
	return photoCount, skillIds, abilityIds, nil
}

func CalcReleaseCounts(ctx context.Context, lteLevel int32, card *pmaster.Card) (map[penum.CardLevelReleaseType]int32, error) {
	cardLevelReleases, err := opmaster.FindCardLevelReleases(
		ctx,
		card.CardLevelReleaseId,
		lteLevel,
	)
	if err != nil {
		return nil, err
	}
	return calcCardLevelRelease(lteLevel, cardLevelReleases), nil
}

func calcCardLevelRelease(lteLevel int32, levelRelease []*pmaster.CardLevelRelease) map[penum.CardLevelReleaseType]int32 {
	releaseMap := make(map[penum.CardLevelReleaseType]int32)
	for _, level := range levelRelease {
		if level.Level > lteLevel {
			continue
		}
		for _, target := range level.Targets {
			number, ok := releaseMap[target.Type]
			if ok && target.Type == penum.CardLevelReleaseType_CardLevelReleaseType_Photo {
				releaseMap[target.Type] = number + target.Number
			} else {
				releaseMap[target.Type] = target.Number
			}
		}
	}
	return releaseMap
}

func convertSkillIds(skillNum int32, card *pmaster.Card) []string {
	skillIds := make([]string, 3)
	if skillNum >= 1 {
		skillIds[0] = card.SkillId1
	}
	if skillNum >= 2 {
		skillIds[1] = card.SkillId2
	}
	if skillNum >= 3 {
		skillIds[2] = card.SkillId3
	}
	return skillIds
}

func convertAbilities(liveAbNum int32, activityNum int32, card *pmaster.Card) []string {
	abilityIds := make([]string, 2)
	if liveAbNum > 0 {
		abilityIds[0] = card.LiveAbilityId
	}
	if activityNum > 0 {
		abilityIds[1] = card.ActivityAbilityId
	}
	return abilityIds
}
