package mcontext

import (
	"context"

	"go.uber.org/zap"
)

type LoggerKey struct{}

func GetLogger(ctx context.Context) *zap.SugaredLogger {
	val := ctx.Value(LoggerKey{})
	logger, ok := val.(*zap.SugaredLogger)
	if !ok {
		return zap.L().Sugar()
	}
	return logger
}

func WithLogger(ctx context.Context, logger *zap.SugaredLogger) context.Context {
	ctx = context.WithValue(ctx, LoggerKey{}, logger)
	return ctx
}
