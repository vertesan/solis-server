package ucommon

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/store/internal/opcommon"
	"github.com/vertesan/solis-server/internal/db/transaction"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ForEachAtomically applies fn to all objects in one transaction when more
// than one write is required, while joining any transaction already in ctx.
func ForEachAtomically[T any](ctx context.Context, objs []*T, fn func(context.Context, *T) error) error {
	if len(objs) == 0 {
		return nil
	}
	if len(objs) == 1 {
		return fn(ctx, objs[0])
	}
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		for _, obj := range objs {
			if err := fn(ctx, obj); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
}

func FindOne[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	opts ...options.Lister[options.FindOneOptions],
) (*T, error) {
	return opcommon.FindOne[T](ctx, depot.User, collection, filter, opts...)
}

func FindAll[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	opts ...options.Lister[options.FindOptions],
) ([]*T, error) {
	return opcommon.FindAll[T](ctx, depot.User, collection, filter, opts...)
}

func InsertOne[T any](
	ctx context.Context,
	collection string,
	obj *T,
	opts ...options.Lister[options.InsertOneOptions],
) error {
	return opcommon.InsertOne(ctx, depot.User, collection, obj, opts...)
}

func UpdateOne[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	obj *T,
	opts ...options.Lister[options.UpdateOneOptions],
) error {
	return opcommon.UpdateOne(ctx, depot.User, collection, filter, obj, opts...)
}

func UpsertOne[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	obj *T,
	opts ...options.Lister[options.UpdateOneOptions],
) error {
	return opcommon.UpsertOne(ctx, depot.User, collection, filter, obj, opts...)
}

func FindAndUpdateOne[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	update map[string]any,
	opts ...options.Lister[options.FindOneAndUpdateOptions],
) (*T, error) {
	return opcommon.FindAndUpdateOne[T](ctx, depot.User, collection, filter, update, opts...)
}

func Delete(
	ctx context.Context,
	collection string,
	filter map[string]any,
) error {
	return opcommon.Delete(ctx, depot.User, collection, filter)
}
