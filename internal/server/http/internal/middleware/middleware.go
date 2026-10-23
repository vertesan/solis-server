package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"
)

func LoggingMiddleware(base *zap.SugaredLogger) fiber.Handler {
	return func(c fiber.Ctx) error {
		logger := base.With(zap.String("api", c.Path()), zap.String("method", c.Method()))
		// Install the request logger before auth/maintenance checks, which can
		// themselves fail and must use the same centralized error handler.
		c.Locals(mcontext.LoggerKey{}, logger)
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
		return c.Next()
	}
}

// ErrorHandler logs the original diagnostic once, then exposes only the HTTP
// status meaning to the client. Register it on Fiber so errors raised before
// routing (for example oversized bodies) follow the same policy as handlers.
func ErrorHandler(base *zap.SugaredLogger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		var httpErr *fiber.Error
		if errors.As(err, &httpErr) && httpErr != nil && httpErr.Code >= 400 && httpErr.Code < 600 {
			code = httpErr.Code
		}

		if c.Locals(mcontext.LoggerKey{}) == nil {
			c.Locals(mcontext.LoggerKey{}, base.With(zap.String("api", c.Path()), zap.String("method", c.Method())))
		}
		mcontext.GetLogger(c).Errorw("HTTP request failed", "status", code, "error", err)

		message := "internal error"
		if code < 500 {
			message = strings.ToLower(http.StatusText(code))
			if message == "" {
				message = "client error"
			}
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.Status(code).SendString(message)
	}
}

func CheckAuth(authorization string) *fiber.Error {
	auth, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok {
		return fiber.NewError(401, "missing or invalid admin Bearer authorization")
	}
	adminAuth, err := config.GetAdminAuth()
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("load admin authorization configuration: %v", err))
	}
	if auth != adminAuth {
		return fiber.NewError(401, "admin Bearer authorization does not match")
	}
	return nil
}
