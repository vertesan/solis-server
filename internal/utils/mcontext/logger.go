package mcontext

import (
	"context"

	"go.uber.org/zap"
)

type LoggerKey struct{}

func GetLogger(ctx context.Context) *zap.Logger {
	val := ctx.Value(LoggerKey{})
	logger, ok := val.(*zap.Logger)
	if !ok {
		return zap.L()
	}
	return logger
}

func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	ctx = context.WithValue(ctx, LoggerKey{}, logger)
	return ctx
}
