package card

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"vertesan/solis-server/db/store/opmaster"
	"vertesan/solis-server/proto/penum"
	"vertesan/solis-server/proto/pmaster"
)

func CalcCardDisplayType(cardId string, rarity int32, evolutionLevel int32) (penum.CardDisplayType, error) {
	cardImages, err := opmaster.FindCardImages(context.Background(), cardId)
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
