package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"
)

func LoggingMiddleware(base *zap.SugaredLogger) fiber.Handler {
	return func(c fiber.Ctx) error {
		logger := base.With(zap.String("api", c.Path()))
		// log API invocation history
		logger.Infof("[%s]", c.Method())
		// if admin APIs are invoked, check authroization first
		if strings.HasPrefix(c.Path(), "/admin/") {
			if err := CheckAuth(c.Get("Authorization")); err != nil {
				return err
			}
		}
		// These operations initialize/promote users using many master tables and
		// must drain alongside gRPC gameplay. Job/status and immutable downloads
		// remain available while publication is in maintenance.
		if strings.HasPrefix(c.Path(), "/admin/user/") || c.Path() == "/getId" || strings.HasPrefix(c.Path(), "/identitytoolkit/") || c.Path() == "/v1/token" {
			leave, accepted := masterdata.Default.Gate.Enter()
			if !accepted {
				return fiber.NewError(fiber.StatusServiceUnavailable, "master data maintenance")
			}
			defer leave()
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
