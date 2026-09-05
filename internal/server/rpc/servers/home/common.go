package home

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

// reconcileNarrativeSchedules unlocks newly satisfied messages and standalone
// telephone calls, then schedules the first waiting entry for each message group.
// Master records are visited in database insertion order.
func reconcileNarrativeSchedules(
	ctx context.Context,
	suid string,
	now int64,
	updated *papi.UpdatedData,
) error {
	return viber.ReconcileNarrativeSchedules(ctx, suid, now, updated, viber.NarrativeUnlockAll, "")
}
