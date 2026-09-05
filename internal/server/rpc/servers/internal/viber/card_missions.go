package viber

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// ReachCharacterCardTrainingMissions uses the highest achieved total, so removing
// and reassigning support cannot repeatedly advance the same training missions.
func ReachCharacterCardTrainingMissions(ctx context.Context, suid, characterID string) ([]*ptransaction.UserMission, error) {
	masters, err := opmaster.FindAllCards(ctx)
	if err != nil {
		return nil, err
	}
	characterCards := make(map[string]bool)
	for _, master := range masters {
		if master.CharacterId == characterID {
			characterCards[master.Id] = true
		}
	}
	cards, err := opuser.FindAllUserCards(ctx, suid)
	if err != nil {
		return nil, err
	}
	var totalLevel, totalSkillLevel int64
	for _, card := range cards {
		if !characterCards[card.CardId] {
			continue
		}
		totalLevel += int64(card.Level)
		for _, skill := range []struct {
			id    string
			level int32
		}{
			{card.SkillId1, card.SkillLevel1}, {card.SkillId2, card.SkillLevel2},
			{card.SkillId3, card.SkillLevel3}, {card.SkillId4, card.SkillLevel4},
		} {
			if skill.id != "" {
				totalSkillLevel += int64(skill.level)
			}
		}
	}
	var updated []*ptransaction.UserMission
	for _, achievement := range []struct {
		typeID penum.MissionType
		value  int64
	}{
		{penum.MissionType_MissionType_CharacterTotalCardLevelAchievement, totalLevel},
		{penum.MissionType_MissionType_CharacterTotalCardSkillLevelAchievement, totalSkillLevel},
	} {
		if achievement.value <= 0 {
			continue
		}
		missions, err := ReachUserMissions(ctx, suid, achievement.typeID, achievement.value, characterID)
		if err != nil {
			return nil, err
		}
		updated = append(updated, missions...)
	}
	return updated, nil
}
