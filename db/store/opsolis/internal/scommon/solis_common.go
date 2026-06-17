package scommon

import (
	"context"
	"vertesan/solis-server/db/internal/depot"
	"vertesan/solis-server/db/store/internal/opcommon"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindOne[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	opts ...options.Lister[options.FindOneOptions],
) (*T, error) {
	return opcommon.FindOne[T](ctx, depot.Solis, collection, filter, opts...)
}

func FindAll[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	opts ...options.Lister[options.FindOptions],
) ([]*T, error) {
	return opcommon.FindAll[T](ctx, depot.Solis, collection, filter, opts...)
}

func InsertOne[T any](
	ctx context.Context,
	collection string,
	obj *T,
	opts ...options.Lister[options.InsertOneOptions],
) error {
	return opcommon.InsertOne(ctx, depot.Solis, collection, obj, opts...)
}

func UpdateOne[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	obj *T,
	opts ...options.Lister[options.UpdateOneOptions],
) error {
	return opcommon.UpdateOne(ctx, depot.Solis, collection, filter, obj, opts...)
}

func UpsertOne[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	obj *T,
	opts ...options.Lister[options.UpdateOneOptions],
) error {
	return opcommon.UpsertOne(ctx, depot.Solis, collection, filter, obj, opts...)
}

func UpsertMany[T any](
	ctx context.Context,
	collection string,
	filter map[string]any,
	objs []*T, opts ...options.Lister[options.UpdateOneOptions],
) error {
	return opcommon.UpsertMany(ctx, depot.Solis, collection, filter, objs, opts...)
}

func Delete(
	ctx context.Context,
	collection string,
	filter map[string]any,
) error {
	return opcommon.Delete(ctx, depot.Solis, collection, filter)
}
