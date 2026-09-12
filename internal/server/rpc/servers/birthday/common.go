package birthday

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func withBirthday(ctx context.Context, fn func(context.Context, string, *birthday.Event) error) error {
	suid := viber.GetSuid(ctx).String()
	now := time.Now()
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		profile, err := opuser.FindUserProfile(ctx, suid)
		if err != nil {
			return nil, err
		}
		event, err := birthday.Reconcile(ctx, suid, profile, now)
		if err != nil {
			return nil, err
		}
		return nil, fn(ctx, suid, event)
	})
	return viber.GameplayError(err)
}
