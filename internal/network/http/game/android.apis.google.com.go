package game

import (
	"fmt"
	"github.com/vertesan/solis-server/internal/network/internal/depot"
	"github.com/vertesan/solis-server/internal/utils/jwto"

	"github.com/gofiber/fiber/v3"
)

var AndroidRegister3 fiber.Handler = func(c fiber.Ctx) error {
	// check header
	pkgId := c.Get("app")
	if pkgId == "" {
		return fiber.NewError(400, "missing app header")
	}
	if pkgId != depot.ServerProfile.PackageName {
		return fiber.NewError(400, "app header mismatches")
	}

	// retrieve X-appid from url encoded form data
	xAppId := c.FormValue("X-appid")
	if xAppId == "" {
		return fiber.NewError(400, "missing x-appid form data")
	}

	apiToken := jwto.CreateAndroidApiToken()

	// set response contents
	c.Set("cache-control", "private, max-age=0")
	c.Set("content-security-policy", "frame-ancestors 'self'")
	return c.SendString(fmt.Sprintf("token=%s:%s", xAppId, apiToken))
}
