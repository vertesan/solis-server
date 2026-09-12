package viber

import (
	"context"
	"slices"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
)

// ReconcileCardSupportLevel must run in the caller's transaction. A card's base
// training, not its assisted level or contract/evolution bonuses, determines the
// five trainers. The fifth trainer can raise the persisted high-water mark, but
// losing/resetting a trainer cannot lower it. User.Get also calls this to repair
// users trained before assistance progression was implemented.
func ReconcileCardSupportLevel(ctx context.Context, suid string, user *ptransaction.User, updated *papi.UpdatedData) ([]*ptransaction.UserCard, error) {
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return nil, err
	}
	if setting.CardSupportLevelLimit < 1 {
		return nil, FailedPreconditionf("master card support level limit is invalid")
	}
	cards, err := opuser.FindAllUserCards(ctx, suid)
	if err != nil {
		return nil, err
	}
	trainers := make([]*ptransaction.UserCard, 0, len(cards))
	for _, card := range cards {
		if !card.Supported {
			trainers = append(trainers, card)
		}
	}
	slices.SortFunc(trainers, func(a, b *ptransaction.UserCard) int {
		if a.BaseLevel != b.BaseLevel {
			if a.BaseLevel > b.BaseLevel {
				return -1
			}
			return 1
		}
		return strings.Compare(a.CardId, b.CardId)
	})
	level := max(user.CardSupportLevel, 1)
	if len(trainers) >= 5 {
		level = max(level, trainers[4].BaseLevel)
	}
	level = min(level, setting.CardSupportLevelLimit)
	if user.CardSupportLevel != level {
		user.CardSupportLevel = level
		if err := opuser.UpsertUser(ctx, suid, user); err != nil {
			return nil, err
		}
		updated.User = user
	}
	characters := make(map[string]bool)
	for _, card := range cards {
		if !card.Supported || card.Level == level {
			continue
		}
		master, err := opmaster.FindCard(ctx, card.CardId)
		if err != nil {
			return nil, err
		}
		if err := usercard.ApplySupportLevel(ctx, master, card, level); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		stories, _, err := UpdateCardStoryUnlocks(ctx, suid, master, level, card.EvolutionLevel)
		if err != nil {
			return nil, err
		}
		updated.Cards = append(updated.Cards, card)
		updated.Stories = append(updated.Stories, stories...)
		characters[master.CharacterId] = true
	}
	for characterID := range characters {
		missions, err := ReachCharacterCardTrainingMissions(ctx, suid, characterID)
		if err != nil {
			return nil, err
		}
		MergeCardTrainingMissions(updated, missions)
	}
	return trainers, nil
}
