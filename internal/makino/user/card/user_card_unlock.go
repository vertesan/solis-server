package card

import (
	"context"
	"fmt"

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
	return unlocksFromReleaseCounts(releaseMap, card)
}

func CalcUnlocksWithEvolution(
	ctx context.Context,
	lteLevel int32,
	evolutionLevel int32,
	card *pmaster.Card,
) (int32, []string, []string, error) {
	releaseMap, err := CalcReleaseCounts(ctx, lteLevel, card)
	if err != nil {
		return 0, nil, nil, err
	}
	releases, err := findCardEvolutionReleases(ctx, evolutionLevel, card)
	if err != nil {
		return 0, nil, nil, err
	}
	applyCardEvolutionReleaseCounts(releaseMap, releases)
	photoCount, skillIDs, abilityIDs, err := unlocksFromReleaseCounts(releaseMap, card)
	if err != nil {
		return 0, nil, nil, err
	}
	for _, release := range releases {
		for _, target := range release.GetTargets() {
			if target.GetType() != penum.CardLevelReleaseType_CardLevelReleaseType_Skill {
				continue
			}
			switch target.GetNumber() {
			case 1:
				skillIDs[0] = card.GetSkillId1()
			case 2:
				skillIDs[1] = card.GetSkillId2()
			case 3:
				skillIDs[2] = card.GetSkillId3()
			case 4:
				skillIDs[3] = card.GetSkillId4()
			default:
				return 0, nil, nil, fmt.Errorf("card evolution release has invalid skill slot %d", target.GetNumber())
			}
		}
	}
	return photoCount, skillIDs, abilityIDs, nil
}

func unlocksFromReleaseCounts(
	releaseMap map[penum.CardLevelReleaseType]int32,
	card *pmaster.Card,
) (int32, []string, []string, error) {
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

func CalcReleaseCountsWithEvolution(
	ctx context.Context,
	lteLevel int32,
	evolutionLevel int32,
	card *pmaster.Card,
) (map[penum.CardLevelReleaseType]int32, error) {
	releaseMap, err := CalcReleaseCounts(ctx, lteLevel, card)
	if err != nil {
		return nil, err
	}
	releases, err := findCardEvolutionReleases(ctx, evolutionLevel, card)
	if err != nil {
		return nil, err
	}
	applyCardEvolutionReleaseCounts(releaseMap, releases)
	return releaseMap, nil
}

func findCardEvolutionReleases(
	ctx context.Context,
	evolutionLevel int32,
	card *pmaster.Card,
) ([]*pmaster.CardEvolutionLevelRelease, error) {
	groupID := card.GetCardEvolutionLevelReleaseGroupId()
	if groupID == "" {
		return nil, nil
	}
	return opmaster.FindCardEvolutionLevelReleases(ctx, groupID, evolutionLevel)
}

func applyCardEvolutionReleaseCounts(
	releaseMap map[penum.CardLevelReleaseType]int32,
	releases []*pmaster.CardEvolutionLevelRelease,
) {
	for _, release := range releases {
		for _, target := range release.GetTargets() {
			switch target.GetType() {
			case penum.CardLevelReleaseType_CardLevelReleaseType_Skill:
				// Evolution skill targets identify an exact slot. They do not
				// backfill ordinary level-gated skill slots.
				continue
			case penum.CardLevelReleaseType_CardLevelReleaseType_Photo:
				releaseMap[target.GetType()] += target.GetNumber()
			default:
				releaseMap[target.GetType()] = target.GetNumber()
			}
		}
	}
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
	skillIds := make([]string, 4)
	if skillNum >= 1 {
		skillIds[0] = card.SkillId1
	}
	if skillNum >= 2 {
		skillIds[1] = card.SkillId2
	}
	if skillNum >= 3 {
		skillIds[2] = card.SkillId3
	}
	if skillNum >= 4 {
		skillIds[3] = card.SkillId4
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
