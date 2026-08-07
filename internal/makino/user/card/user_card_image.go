package card

import (
	"cmp"
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"slices"
)

func CalcCardDisplayType(ctx context.Context, cardId string, rarity int32, evolutionLevel int32) (penum.CardDisplayType, error) {
	cardImages, err := opmaster.FindCardImages(ctx, cardId)
	if err != nil {
		return penum.CardDisplayType_CardDisplayType_Unknown, err
	}
	switch len(cardImages) {
	case 0:
		return penum.CardDisplayType_CardDisplayType_Unknown, fmt.Errorf("no cardImage master data for %q", cardId)
	case 2:
		if evolutionLevel > 0 {
			return penum.CardDisplayType_CardDisplayType_AfterEvolution, nil
		}
		if rarity >= 5 {
			return penum.CardDisplayType_CardDisplayType_AfterLimitBreak, nil
		}
		min := slices.MinFunc(cardImages, func(a, b *pmaster.CardImage) int {
			return cmp.Compare(a.DisplayType, b.DisplayType)
		})
		return penum.CardDisplayType(min.DisplayType), nil
	default:
		return penum.CardDisplayType_CardDisplayType_AfterLimitBreak, nil
	}
}
