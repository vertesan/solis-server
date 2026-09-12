package card

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// ApplySupportLevel changes only the assisted projection. Base training and EXP
// remain intact so removing assistance can restore them exactly. The assist
// level is independent of the card's own rarity/contract level limit.
func ApplySupportLevel(ctx context.Context, master *pmaster.Card, card *ptransaction.UserCard, level int32) error {
	attributes, err := CalcUserCardBaseAttr(ctx, master, level, card.Rarity)
	if err != nil {
		return err
	}
	photos, skills, abilities, err := CalcUnlocksWithEvolution(ctx, level, card.EvolutionLevel, master)
	if err != nil {
		return err
	}
	card.Level = level
	card.Vocal, card.Dance, card.Visual, card.Stamina = attributes.Vocal, attributes.Dance, attributes.Visual, attributes.Stamina
	card.PhotoEquipableCount = photos
	card.SkillId1, card.SkillId2, card.SkillId3, card.SkillId4 = skills[0], skills[1], skills[2], skills[3]
	card.LiveAbilityId, card.ActivityAbilityId = abilities[0], abilities[1]
	if card.SkillId4 != "" && card.SkillLevel4 == 0 {
		card.SkillLevel4 = 1
	}
	return nil
}
