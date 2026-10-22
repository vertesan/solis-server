package viber

import (
	"context"
	"strings"

	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

// ConditionEvaluator only relaxes absolute official dates for an explicitly
// selected private-server rotation. All other user prerequisites still apply.
func (catalog *GachaMasterCatalog) ConditionEvaluator(ctx context.Context, suid string, now int64, gachaID string) *CommerceConditionEvaluator {
	evaluator := NewCommerceConditionEvaluator(ctx, suid, now)
	evaluator.ignoreOfficialDates = catalog.IgnoresDates(gachaID)
	return evaluator
}

func (catalog *GachaMasterCatalog) IsEnabled(gachaID string) bool {
	_, enabled := catalog.enabledGachaIds[gachaID]
	return enabled
}

func (catalog *GachaMasterCatalog) IgnoresDates(gachaID string) bool {
	_, explicit := catalog.explicitGachaIds[gachaID]
	return explicit && catalog.IgnoreOfficialDates
}

// IsGachaEnabled keeps the permanent card catalog small without changing item,
// premium-reward, or showcase-toy gachas. This policy applies before user rows
// are initialized as well as when existing accounts refresh their catalog.
func IsGachaEnabled(gacha *pmaster.Gacha) bool {
	if gacha == nil {
		return false
	}
	switch gacha.GetGachaType() {
	case penum.GachaType_GachaType_Normal, penum.GachaType_GachaType_Continuous, penum.GachaType_GachaType_Stamp:
	default:
		return true
	}
	if gacha.GetIsPeriodDisplay() {
		return false
	}
	// Master data has no separate beginner/birthday/fixed-pool flags.
	// Some campaign ticket gachas also hide their period and are gated only on
	// ticket ownership, so a missing end date does not make them permanent.
	id, name := gacha.GetId(), gacha.GetName()
	return !strings.Contains(id, "beginner") &&
		!strings.Contains(id, "birthday") &&
		!strings.Contains(id, "fixed") &&
		!strings.HasPrefix(id, "gacha-free-") &&
		!strings.HasPrefix(id, "gacha-continuous-") &&
		!strings.HasSuffix(id, "-free") &&
		!strings.Contains(name, "初心者") &&
		!strings.Contains(name, "スタート") &&
		!strings.Contains(name, "誕生日")
}
