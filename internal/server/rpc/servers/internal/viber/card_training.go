package viber

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// MergeCardTrainingMissions publishes the final state when one enhancement also
// advances the same character's missions through its assisted cards.
func MergeCardTrainingMissions(updated *papi.UpdatedData, missions []*ptransaction.UserMission) {
	for _, mission := range missions {
		found := false
		for index, existing := range updated.Missions {
			if existing.MissionId == mission.MissionId {
				updated.Missions[index] = mission
				found = true
				break
			}
		}
		if !found {
			updated.Missions = append(updated.Missions, mission)
		}
	}
}

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

// UpdateCardStoryUnlocks keeps read progress when a reset or support change
// relocks a story. Reaching the required level reactivates the same saved row.
func UpdateCardStoryUnlocks(ctx context.Context, suid string, master *pmaster.Card, level, evolutionLevel int32) (unlocked, locked []*ptransaction.UserStory, err error) {
	releases, err := usercard.CalcReleaseCountsWithEvolution(ctx, level, evolutionLevel, master)
	if err != nil {
		return nil, nil, err
	}
	count := releases[penum.CardLevelReleaseType_CardLevelReleaseType_Story]
	for i, entry := range master.GetStories() {
		available := int32(i) < count
		story, err := opuser.FindUserStory(ctx, suid, entry.StoryId)
		if errors.Is(err, mongo.ErrNoDocuments) {
			if !available {
				continue
			}
			story = &ptransaction.UserStory{StoryId: entry.StoryId, StatusType: penum.StoryStatusType_StoryStatusType_Unread}
		} else if err != nil {
			return nil, nil, err
		} else if story.IsInvalid == !available {
			continue
		}
		story.IsInvalid = !available
		if err := opuser.UpsertUserStory(ctx, suid, story); err != nil {
			return nil, nil, err
		}
		if available {
			unlocked = append(unlocked, story)
		} else {
			locked = append(locked, story)
		}
	}
	return unlocked, locked, nil
}
