package diver

import (
	"strings"
	"vertesan/solis-server/utils/config"
	"vertesan/solis-server/utils/rich"

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

func CheckAuth(authorization string) error {
	auth, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok {
		return fiber.NewError(401, "unauthorized")
	}
	if auth != config.GetAdminAuth() {
		return fiber.NewError(401, "unauthorized")
	}
	return nil
}
