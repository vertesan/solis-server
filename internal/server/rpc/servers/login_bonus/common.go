package loginbonus

import (
	"strings"

	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

func loginBonusEnabled(master *pmaster.LoginBonus) bool {
	// Start-dash bonuses have the ordinary Event type, not their own enum.
	// Use the master name so both installments (and future ones) are excluded.
	return !strings.Contains(master.GetName(), "スタートダッシュ")
}
