package mcommon

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"iter"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type MongoMaster interface {
	GetData() []byte
}

func FindOne[T interface{}, U interface{},
	W interface {
		*T
		MongoMaster
	},
	V interface {
		*U
		proto.Message
	},
](
	ctx context.Context,
	dbName string,
	filter interface{},
) (V, error) {
	data := W(new(T))
	err := depot.Master.Collection(dbName).FindOne(
		ctx, filter,
	).Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode value from master db: %w", err)
	}
	pit := V(new(U))
	if err := proto.Unmarshal(data.GetData(), pit); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bytes to proto object: %w", err)
	}
	return pit, nil
}

func FindMany[T interface{}, U interface{},
	W interface {
		*T
		MongoMaster
	},
	V interface {
		*U
		proto.Message
	},
](
	ctx context.Context,
	dbName string,
	filter interface{},
	opts ...options.Lister[options.FindOptions],
) ([]V, error) {
	src := []W{}
	cursor, err := depot.Master.Collection(dbName).Find(
		ctx, filter, opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get all protos from master db: %w", err)
	}
	if err := cursor.All(ctx, &src); err != nil {
		return nil, fmt.Errorf("failed to decode mongo object: %w", err)
	}
	dst := make([]V, 0, len(src))
	for _, it := range src {
		pit := V(new(U))
		if err := proto.Unmarshal(it.GetData(), pit); err != nil {
			return nil, fmt.Errorf("failed to unmarshal bytes to proto: %w", err)
		}
		dst = append(dst, pit)
	}
	return dst, nil
}

func YieldMany[T interface{}, U interface{},
	W interface {
		*T
		MongoMaster
	},
	V interface {
		*U
		proto.Message
	},
](
	ctx context.Context,
	dbName string,
	filter interface{},
	opts ...options.Lister[options.FindOptions],
) iter.Seq[V] {
	return func(yield func(V) bool) {
		logger := mcontext.GetLogger(ctx)
		cursor, err := depot.Master.Collection(dbName).Find(
			ctx, filter, opts...,
		)
		if err != nil {
			logger.Error("failed to get protos from master db", zap.Error(err))
			return
		}
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			obj := new(W)
			if err := cursor.Decode(obj); err != nil {
				logger.Error("failed to decode mongo object", zap.Error(err))
				return
			}
			p := V(new(U))
			if err := proto.Unmarshal((*obj).GetData(), p); err != nil {
				logger.Error("failed to unmarshal bytes to proto", zap.Error(err))
				return
			}
			if !yield(p) {
				return
			}
		}
		if err := cursor.Err(); err != nil {
			logger.Error("failed while reading protos from master db", zap.Error(err))
		}
	}
}
