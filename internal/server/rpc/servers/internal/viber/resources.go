package viber

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ConsumePermanentItem(ctx context.Context, suid, itemID string, amount int64) (*ptransaction.UserItem, error) {
	if amount <= 0 {
		return nil, status.Error(codes.Internal, "consumption amount must be positive")
	}
	items, err := opuser.FindUserItems(ctx, suid, itemID)
	if err != nil {
		return nil, err
	}
	var item *ptransaction.UserItem
	for _, candidate := range items {
		if candidate.ExpiredTime == 0 {
			item = candidate
			break
		}
	}
	if item == nil || item.Amount < amount {
		return nil, FailedPreconditionf("insufficient %s", itemID)
	}
	item.Amount -= amount
	if err := opuser.UpsertUserItem(ctx, suid, item); err != nil {
		return nil, err
	}
	return item, nil
}

func ConsumeGold(ctx context.Context, suid string, amount int64) (*ptransaction.UserPoint, error) {
	if amount <= 0 {
		return nil, status.Error(codes.Internal, "gold consumption amount must be positive")
	}
	point, err := opuser.FindUserPoint(ctx, suid, penum.PointType_PointType_Gold)
	if err != nil {
		return nil, err
	}
	if point.Amount < amount {
		return nil, FailedPreconditionf("insufficient gold")
	}
	point.Amount -= amount
	if err := opuser.UpsertUserPoint(ctx, suid, point); err != nil {
		return nil, err
	}
	return point, nil
}

func InvalidArgumentf(format string, args ...any) error {
	return status.Errorf(codes.InvalidArgument, format, args...)
}

func FailedPreconditionf(format string, args ...any) error {
	return status.Errorf(codes.FailedPrecondition, format, args...)
}

func GameplayError(err error) error {
	if err == nil || status.Code(err) != codes.Unknown {
		return err
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return status.Error(codes.NotFound, "requested game data was not found")
	}
	return err
}
