package http

import (
	"context"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/server/http/admin"
	"github.com/vertesan/solis-server/internal/server/http/game"
	"github.com/vertesan/solis-server/internal/server/http/internal/snippet"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/rich"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
)

func StartHTTP(ctx context.Context) {
	app := fiber.New(fiber.Config{
		AppName:      "solis-server-http",
		ReadTimeout:  time.Second * 30,
		WriteTimeout: time.Second * 30,
		IdleTimeout:  time.Second * 60,
		JSONEncoder:  json.Marshal,
		JSONDecoder:  json.Unmarshal,
	})

	// logging middleware
	app.Use(middleware)

	app.Get("/getId", game.QseedGetId).Name("id.qseed.jp/getId")
	app.Post("/v1/token", game.FirebaseToken).Name("securetoken.googleapis.com/v1/token")
	app.Post("/identitytoolkit/v3/relyingparty/verifyCustomToken", game.FirebaseVerifyCustomToken).Name("www.googleapis.com/identitytoolkit/v3/relyingparty/verifyCustomToken")
	app.Post("/identitytoolkit/v3/relyingparty/getAccountInfo", game.FirebaseGetAccountInfo).Name("www.googleapis.com/identitytoolkit/v3/relyingparty/getAccountInfo")
	app.Post("/v1/projects/solis-firebase/installations", game.FirebaseInstallations).Name("firebaseinstallations.googleapis.com/v1/projects/solis-firebase/installations")
	app.Post("/c2dm/register3", game.AndroidRegister3).Name("android.apis.google.com/c2dm/register3")
	app.Get(`/:db/:filename<regex([a-z0-9]{64})>`, game.GetMaster).Name("climaster.game-idolypride.jp")
	app.Get("/v2/pub/a/212/v/:someid/list/:fromv", game.GetManifest).Name("api.octo-cloud.com")
	app.Get(`/solis-:revision<regex(\d{6})>-:type<alpha>/:object`, game.GetAsset).Name("d2ilil7yh5oi1v.cloudfront.net/solis")

	// admin
	app.Post("/admin/user/create", admin.CreateUser)
	app.Post("/admin/user/delete", admin.DeleteUser)

	// catch all for debug purpose
	app.All("*", func(c fiber.Ctx) error {
		rich.Warn("unhandled request: [%s] %s", c.Method(), c.Path())
		return fiber.NewError(404, "not found")
	})

	port := config.GetHttpPort()
	rich.Info("http server listening at port: %v", port)
	app.Listen(":"+port, fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx,
	})
}

var middleware fiber.Handler = func(c fiber.Ctx) error {
	// log API invocation history
	rich.Info("[%s] %s", c.Method(), c.Path())
	// if admin APIs are invoked, check authroization first
	if strings.HasPrefix(c.Path(), "/admin/") {
		if err := snippet.CheckAuth(c.Get("Authorization")); err != nil {
			return err
		}
	}

	return c.Next()
}
