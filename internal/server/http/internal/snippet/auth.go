package snippet

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func CheckAuth(authorization string) *fiber.Error {
	auth, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok {
		return fiber.NewError(401, "unauthorized")
	}
	if auth != config.GetAdminAuth() {
		return fiber.NewError(401, "unauthorized")
	}
	return nil
}
