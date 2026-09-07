package viber

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// ValidateCardDisplayType checks explicit artwork selections for both the card
// itself and profile backgrounds. Profile's automatic display (zero) is handled
// separately by its caller.
func ValidateCardDisplayType(ctx context.Context, card *ptransaction.UserCard, displayType penum.CardDisplayType) error {
	if displayType < penum.CardDisplayType_CardDisplayType_BeforeLimitBreak || displayType > penum.CardDisplayType_CardDisplayType_AfterEvolution {
		return InvalidArgumentf("a valid displayType is required")
	}
	images, err := opmaster.FindCardImages(ctx, card.GetCardId())
	if err != nil {
		return err
	}
	available := false
	for _, image := range images {
		if image.DisplayType == displayType {
			available = true
			break
		}
	}
	if !available {
		return FailedPreconditionf("card has no image for the requested display type")
	}
	if displayType == penum.CardDisplayType_CardDisplayType_AfterLimitBreak && card.GetRarity() < 5 {
		return FailedPreconditionf("card must reach rarity 5 for its limit-break image")
	}
	if displayType == penum.CardDisplayType_CardDisplayType_AfterEvolution && card.GetEvolutionLevel() <= 0 {
		return FailedPreconditionf("card must be evolved for its evolution image")
	}
	return nil
}
