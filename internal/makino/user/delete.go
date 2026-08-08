package user

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
)

func DeleteUser(ctx context.Context, suid string) error {
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, opuser.DeleteAllUserData(ctx, suid)
	})
}
