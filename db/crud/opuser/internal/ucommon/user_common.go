package ucommon

import (
	"context"
	"vertesan/solis-server/db/crud/internal/opcommon"
	"vertesan/solis-server/db/internal/depot"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

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

func UpsertMany[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	objs []*T, opts ...options.Lister[options.UpdateOneOptions],
) error {
	return opcommon.UpsertMany(ctx, depot.User, collection, filter, objs, opts...)
}

func Delete(
	ctx context.Context,
	collection string,
	filter map[string]any,
) error {
	return opcommon.Delete(ctx, depot.User, collection, filter)
}
