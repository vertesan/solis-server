package viber

import (
	"context"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AddManagerExp shares the same level and dedicated-photo unlock rules for
// idol work and mission rewards. The caller owns the surrounding transaction.
func AddManagerExp(ctx context.Context, suid string, amount int64, updated *papi.UpdatedData) (*ptransaction.User, error) {
	if amount <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "manager exp must be positive")
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	if user.GetManagerExp() > math.MaxInt64-amount {
		return nil, status.Error(codes.FailedPrecondition, "manager exp overflows")
	}
	user.ManagerExp += amount
	maxLevel, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		return nil, err
	}
	for user.GetManagerLevel() < maxLevel.GetLevel() {
		next, err := opmaster.FindManagerLevel(ctx, user.GetManagerLevel()+1)
		if err != nil {
			return nil, err
		}
		if user.GetManagerExp() < next.GetRequiredExp() {
			break
		}
		user.ManagerLevel = next.GetLevel()
	}
	if err := opuser.UpsertUser(ctx, suid, user); err != nil {
		return nil, err
	}
	updated.User = user
	unlockedPhotos, err := UnlockManagerLevelPhotoAbilities(ctx, suid, user.GetManagerLevel())
	if err != nil {
		return nil, err
	}
	updated.Photos = append(updated.Photos, unlockedPhotos...)
	if err := salary.RefreshSnapshot(ctx, suid, time.Now()); err != nil {
		return nil, err
	}
	return user, nil
}
