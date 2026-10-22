package viber

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
)

type GachaRules = smodel.GachaRules
type GachaRarityPool = smodel.GachaRarityPool

type gachaRulesContextKey struct{}

// WithGachaRules shares one rules snapshot within a request, including large
// multi-draws, without delaying catalog changes for later requests.
func WithGachaRules(ctx context.Context, rules *GachaRules) context.Context {
	return context.WithValue(ctx, gachaRulesContextKey{}, rules)
}

// LoadGachaRules reads the current administrator-managed rules. Never cache the
// result globally: edits must take effect without restarting the game server.
func LoadGachaRules(ctx context.Context) (*GachaRules, error) {
	if rules, ok := ctx.Value(gachaRulesContextKey{}).(*GachaRules); ok && rules != nil {
		return rules, nil
	}
	rules, err := opsolis.FindGachaRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("load gacha rules: %w", err)
	}
	if err := rules.Validate(); err != nil {
		return nil, err
	}
	return rules, nil
}
