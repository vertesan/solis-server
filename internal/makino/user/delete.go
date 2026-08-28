package user

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func DeleteUser(ctx context.Context, suid string) error {
	auth, err := opuser.GetUserAuthByServerId(ctx, suid)
	if err != nil {
		return err
	}
	if !validPhotoDirectoryPublicUserId(auth.PublicUserId) {
		return fmt.Errorf("invalid public user ID %q while deleting user photos", auth.PublicUserId)
	}

	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, opuser.DeleteAllUserData(ctx, suid)
	}); err != nil {
		return err
	}

	photoDirectory := filepath.Join(config.GetResourceDir(), "Photo", auth.PublicUserId)
	if err := os.RemoveAll(photoDirectory); err != nil {
		return fmt.Errorf("user database records were deleted, but photo files could not be removed: %w", err)
	}
	return nil
}

func validPhotoDirectoryPublicUserId(value string) bool {
	if len(value) != 8 {
		return false
	}
	for i := range len(value) {
		if (value[i] < 'A' || value[i] > 'Z') && (value[i] < '0' || value[i] > '9') {
			return false
		}
	}
	return true
}
