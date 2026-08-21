package card

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// ApplyDuplicateEvolution applies one duplicate's master-defined evolution
// EXP to card. It returns the absolute level reached by this occurrence, or
// zero when no level boundary was crossed.
func ApplyDuplicateEvolution(
	ctx context.Context,
	masterCard *pmaster.Card,
	card *ptransaction.UserCard,
) (int32, error) {
	groupID := masterCard.GetCardEvolutionLevelGroupId()
	if groupID == "" {
		return 0, nil
	}
	levels, err := loadCardEvolutionLevels(ctx, groupID)
	if err != nil {
		return 0, err
	}

	currentIndex := -1
	for index, level := range levels {
		if level.GetLevel() == card.GetEvolutionLevel() {
			currentIndex = index
			break
		}
	}
	if currentIndex < 0 {
		return 0, fmt.Errorf("card %q evolution level %d is absent from group %q", card.GetCardId(), card.GetEvolutionLevel(), groupID)
	}
	totalExp := card.GetEvolutionLevelTotalExp()
	current := levels[currentIndex]
	maxExp := int64(levels[len(levels)-1].GetExp())
	if totalExp < int64(current.GetExp()) || totalExp > maxExp ||
		currentIndex+1 < len(levels) && totalExp >= int64(levels[currentIndex+1].GetExp()) {
		return 0, fmt.Errorf("card %q evolution EXP is inconsistent with level %d", card.GetCardId(), card.GetEvolutionLevel())
	}

	gain := int64(current.GetDuplicateObtainExp())
	if gain >= maxExp-totalExp {
		totalExp = maxExp
	} else {
		totalExp += gain
	}
	beforeLevel := card.GetEvolutionLevel()
	afterLevel := beforeLevel
	for _, level := range levels {
		if int64(level.GetExp()) <= totalExp {
			afterLevel = level.GetLevel()
		}
	}
	card.EvolutionLevelTotalExp = totalExp
	card.EvolutionLevel = afterLevel
	if afterLevel > beforeLevel {
		return afterLevel, nil
	}
	return 0, nil
}

func InitialEvolutionState(ctx context.Context, masterCard *pmaster.Card) (int32, int64, error) {
	groupID := masterCard.GetCardEvolutionLevelGroupId()
	if groupID == "" {
		return 0, 0, nil
	}
	levels, err := loadCardEvolutionLevels(ctx, groupID)
	if err != nil {
		return 0, 0, err
	}
	return levels[0].GetLevel(), int64(levels[0].GetExp()), nil
}

func loadCardEvolutionLevels(ctx context.Context, groupID string) ([]*pmaster.CardEvolutionLevel, error) {
	levels, err := opmaster.FindCardEvolutionLevels(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(levels) == 0 {
		return nil, fmt.Errorf("card evolution level group %q is empty", groupID)
	}
	for index, level := range levels {
		if level.GetGroupId() != groupID || level.GetLevel() < 0 || level.GetExp() < 0 || level.GetDuplicateObtainExp() < 0 {
			return nil, fmt.Errorf("card evolution level group %q is invalid", groupID)
		}
		if index > 0 && (level.GetLevel() <= levels[index-1].GetLevel() || level.GetExp() <= levels[index-1].GetExp()) {
			return nil, fmt.Errorf("card evolution level group %q is not strictly increasing", groupID)
		}
	}
	return levels, nil
}
