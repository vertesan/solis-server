package db

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/internal/update"
)

// CheckMasterDataUpdate synchronizes master data with the configured official
// manifest. It is a startup operation: callers must have connected MongoDB and
// must not serve gameplay requests until it finishes.
func CheckMasterDataUpdate(ctx context.Context) error {
	return update.MasterDataUpdateCheck(ctx)
}
