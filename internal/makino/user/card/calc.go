package card

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

type Attributes struct {
	Vocal   int64
	Dance   int64
	Visual  int64
	Stamina int64
}

func CalcUserCardBaseAttr(
	ctx context.Context,
	card *pmaster.Card,
	level int32,
	rarity int32,
) (*Attributes, error) {
	cardParameter, err := opmaster.FindCardParameter(
		ctx,
		card.CardParameterId,
		int64(level),
	)
	if err != nil {
		return nil, err
	}
	cardRarity, err := opmaster.FindCardRarity(ctx, int64(rarity))
	if err != nil {
		return nil, err
	}
	attr := &Attributes{
		Vocal:   calcAttr(cardParameter.Value, card.GetVocalRatioPermil(), cardRarity.GetParameterBonusPermil()),
		Dance:   calcAttr(cardParameter.Value, card.GetDanceRatioPermil(), cardRarity.GetParameterBonusPermil()),
		Visual:  calcAttr(cardParameter.Value, card.GetVisualRatioPermil(), cardRarity.GetParameterBonusPermil()),
		Stamina: calcAttr(cardParameter.StaminaValue, card.GetStaminaRatioPermil(), cardRarity.GetParameterBonusPermil()),
	}
	return attr, nil
}

func CalcUserCardBaseAttrByCardId(
	ctx context.Context,
	cardId string,
	level int32,
	rarity int32,
) (*Attributes, error) {
	card, err := opmaster.FindCard(ctx, cardId)
	if err != nil {
		return nil, err
	}
	return CalcUserCardBaseAttr(ctx, card, level, rarity)
}

// calculate card base attribute.
func calcAttr(param int64, ratio int32, bonus int32) int64 {
	return param * int64(ratio) / 1000 * int64(bonus) / 1000
}
