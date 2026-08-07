package diver

import (
	"context"

	"github.com/vertesan/solis-server/internal/utils/mcontext"

	"github.com/gofiber/fiber/v3"
)

func NewFiberError(ctx context.Context, code int, message ...any) *fiber.Error {
	logger := mcontext.GetLogger(ctx)
	l := len(message)
	if l == 1 {
		logger.Error(message[0].(string))
	} else if l > 1 {
		logger.Sugar().Errorf(message[0].(string), message[1:]...)
	}

	return fiber.NewErrorf(code, message...)
}
