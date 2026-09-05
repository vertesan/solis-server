package viber

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// ReachCharacterHomeTalkMissions projects distinct first-read history into
// character conversation achievements. An empty characterID reconciles all
// characters, including talks read before their Company missions were unlocked.
func ReachCharacterHomeTalkMissions(ctx context.Context, suid, characterID string) ([]*ptransaction.UserMission, error) {
	masters, err := opmaster.FindAllHomeTalks(ctx)
	if err != nil {
		return nil, err
	}
	characterByTalk := make(map[string]string, len(masters))
	for _, master := range masters {
		if master.GetCharacterId() != "" && (characterID == "" || master.GetCharacterId() == characterID) {
			characterByTalk[master.GetHomeTalkId()] = master.GetCharacterId()
		}
	}
	talks, err := opuser.FindAllUserHomeTalks(ctx, suid)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(talks))
	counts := make(map[string]int64)
	for _, talk := range talks {
		id := talk.GetHomeTalkId()
		character := characterByTalk[id]
		if _, duplicate := seen[id]; duplicate || character == "" {
			continue
		}
		seen[id] = struct{}{}
		counts[character]++
	}
	characters := make([]string, 0, len(counts))
	for character := range counts {
		characters = append(characters, character)
	}
	sort.Strings(characters)
	var updated []*ptransaction.UserMission
	for _, character := range characters {
		missions, err := ReachUserMissions(ctx, suid, penum.MissionType_MissionType_CharacterHomeTalkReadAchievement, counts[character], character)
		if err != nil {
			return nil, err
		}
		updated = append(updated, missions...)
	}
	return updated, nil
}
