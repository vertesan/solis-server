package diver

import (
	"github.com/vertesan/solis-server/internal/utils/rich"

	"github.com/gofiber/fiber/v3"
)

func NewFiberError(code int, message ...any) *fiber.Error {
	l := len(message)
	if l == 1 {
		rich.Error(message[0].(string))
	} else if l > 1 {
		rich.Error(message[0].(string), message[1:]...)
	}

	return fiber.NewErrorf(code, message...)
}
