package opcommon

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindOne[T any](
	ctx context.Context,
	database *mongo.Database,
	collection string,
	filter map[string]any,
	opts ...options.Lister[options.FindOneOptions],
) (*T, error) {
	obj := new(T)
	if err := database.Collection(collection).FindOne(ctx, filter, opts...).Decode(obj); err != nil {
		return nil, fmt.Errorf("failed to FindOne for [%s].[%s]: %w", database.Name(), collection, err)
	}
	return obj, nil
}

func FindAll[T any](
	ctx context.Context,
	database *mongo.Database,
	collection string,
	filter map[string]any,
	opts ...options.Lister[options.FindOptions],
) ([]*T, error) {
	var objs []*T
	cursor, err := database.Collection(collection).Find(ctx, filter, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to FindAll for [%s].[%s]: %w", database.Name(), collection, err)
	}
	if err := cursor.All(ctx, &objs); err != nil {
		return nil, fmt.Errorf("failed to decode document for [%s].[%s]: %w", database.Name(), collection, err)
	}
	return objs, nil
}

func InsertOne[T any](
	ctx context.Context,
	database *mongo.Database,
	collection string,
	obj *T,
	opts ...options.Lister[options.InsertOneOptions],
) error {
	if _, err := database.Collection(collection).InsertOne(
		ctx,
		obj,
		opts...,
	); err != nil {
		return fmt.Errorf("failed to InsertOne for [%s].[%s]: %w", database.Name(), collection, err)
	}
	return nil
}

func UpdateOne[T any](
	ctx context.Context,
	database *mongo.Database,
	collection string,
	filter map[string]any,
	obj *T,
	opts ...options.Lister[options.UpdateOneOptions],
) error {
	if _, err := database.Collection(collection).UpdateOne(
		ctx,
		filter,
		bson.M{"$set": obj},
		opts...,
	); err != nil {
		return fmt.Errorf("failed to UpdateOne for [%s].[%s]: %w", database.Name(), collection, err)
	}
	return nil
}

func UpsertOne[T any](
	ctx context.Context,
	database *mongo.Database,
	collection string,
	filter map[string]any,
	obj *T,
	opts ...options.Lister[options.UpdateOneOptions],
) error {
	if _, err := database.Collection(collection).UpdateOne(
		ctx,
		filter,
		bson.M{"$set": obj},
		append(opts, options.UpdateOne().SetUpsert(true))...,
	); err != nil {
		return fmt.Errorf("failed to UpsertOne for [%s].[%s]: %w", database.Name(), collection, err)
	}
	return nil
}

func UpsertMany[T any](
	ctx context.Context,
	database *mongo.Database,
	collection string,
	filter map[string]any,
	objs []*T, opts ...options.Lister[options.UpdateOneOptions],
) error {
	for _, obj := range objs {
		UpsertOne(ctx, database, collection, filter, obj, opts...)
	}
	return nil
}

func Delete(
	ctx context.Context,
	database *mongo.Database,
	collection string,
	filter map[string]any,
) error {
	if _, err := database.Collection(collection).DeleteMany(ctx, filter); err != nil {
		return fmt.Errorf("failed to Delete for [%s].[%s]: %w", database.Name(), collection, err)
	}
	return nil
}
