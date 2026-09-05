package excursion

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func requireExcursionUnlocked(ctx context.Context, suid string) error {
	lock, err := opmaster.FindFunctionLock(ctx, penum.FunctionLockType_FunctionLockType_Excursion)
	if err != nil {
		return err
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return err
	}
	if user.GetManagerLevel() < lock.GetRequiredManagerLevel() {
		return status.Error(codes.PermissionDenied, "excursion is locked")
	}
	unlocked, err := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli()).Satisfied(lock.GetUnlockConditionId())
	if err != nil {
		return err
	}
	if !unlocked {
		return status.Error(codes.PermissionDenied, "excursion is locked")
	}
	return nil
}

func findExcursionStateOrEmpty(ctx context.Context, suid string) (*umodel.UserExcursionState, error) {
	state, err := opuser.FindUserExcursionState(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		state = &umodel.UserExcursionState{SUID: suid, OfferedPlaceIds: []string{}}
	}
	return state, nil
}
