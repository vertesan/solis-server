package moshikoi

import (
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

// VisibleLove keeps negative route scores in storage for reversible accounting,
// but exposes the lowest heroine tier (zero) to the client. LoveHomeAction's
// master ranges start at zero; clamping the stored score instead would let a
// player gain favorability by repeatedly selecting a negative choice and reset.
func VisibleLove(love *ptransaction.UserLove) *ptransaction.UserLove {
	if love == nil || love.GetHiddenFavorability() >= 0 {
		return love
	}
	visible := proto.Clone(love).(*ptransaction.UserLove)
	visible.HiddenFavorability = 0
	return visible
}
