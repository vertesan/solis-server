package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"
)

func LoggingMiddleware(base *zap.SugaredLogger) fiber.Handler {
	return func(c fiber.Ctx) error {
		logger := base.With(zap.String("api", c.Path()))
		// log API invocation history
		logger.Infof("[%s] %s", c.Method(), c.Path())
		// if admin APIs are invoked, check authroization first
		if strings.HasPrefix(c.Path(), "/admin/") {
			if err := CheckAuth(c.Get("Authorization")); err != nil {
				return err
			}
		}
		c.Locals(mcontext.LoggerKey{}, logger)
		return c.Next()
	}
}

func CheckAuth(authorization string) *fiber.Error {
	auth, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok {
		return fiber.NewError(401, "unauthorized")
	}
	adminAuth, err := config.GetAdminAuth()
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	if auth != adminAuth {
		return fiber.NewError(401, "unauthorized")
	}
	return nil
}
