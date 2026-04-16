package card

import (
	"context"
	"vertesan/solis-server/db/crud/opmaster"
	"vertesan/solis-server/proto/pmaster"
)

type Attributes struct {
	Vocal   int64
	Dance   int64
	Visual  int64
	Stamina int64
}

func CalcUserCardBaseAttr(
	card *pmaster.Card,
	level int32,
	rarity int32,
) (*Attributes, error) {
	cardParameter, err := opmaster.FindCardParameter(
		context.Background(),
		card.CardParameterId,
		int64(level),
	)
	if err != nil {
		return nil, err
	}
	cardRarity, err := opmaster.FindCardRarity(context.Background(), int64(rarity))
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
	cardId string,
	level int32,
	rarity int32,
) (*Attributes, error) {
	card, err := opmaster.FindCard(context.Background(), cardId)
	if err != nil {
		return nil, err
	}
	return CalcUserCardBaseAttr(card, level, rarity)
}

// calculate card base attribute.
func calcAttr(param int64, ratio int32, bonus int32) int64 {
	return param * int64(ratio) / 1000 * int64(bonus) / 1000
}
