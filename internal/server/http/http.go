package http

import (
	"context"
	"fmt"
	"time"

	"github.com/vertesan/solis-server/internal/server/http/admin"
	"github.com/vertesan/solis-server/internal/server/http/game"
	"github.com/vertesan/solis-server/internal/server/http/internal/middleware"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.uber.org/zap"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
)

func StartHTTP(ctx context.Context, base *zap.SugaredLogger) error {
	app := fiber.New(fiber.Config{
		AppName:      "solis-server-http",
		ReadTimeout:  time.Second * 30,
		WriteTimeout: time.Second * 30,
		IdleTimeout:  time.Second * 60,
		JSONEncoder:  json.Marshal,
		JSONDecoder:  json.Unmarshal,
		ErrorHandler: middleware.ErrorHandler(base),
	})

	// logging middleware
	app.Use(middleware.LoggingMiddleware(base))

	app.Get("/getId", game.QseedGetId).Name("id.qseed.jp/getId")
	app.Post("/v1/token", game.FirebaseToken).Name("securetoken.googleapis.com/v1/token")
	app.Post("/identitytoolkit/v3/relyingparty/verifyCustomToken", game.FirebaseVerifyCustomToken).Name("www.googleapis.com/identitytoolkit/v3/relyingparty/verifyCustomToken")
	app.Post("/identitytoolkit/v3/relyingparty/getAccountInfo", game.FirebaseGetAccountInfo).Name("www.googleapis.com/identitytoolkit/v3/relyingparty/getAccountInfo")
	app.Post("/v1/projects/solis-firebase/installations", game.FirebaseInstallations).Name("firebaseinstallations.googleapis.com/v1/projects/solis-firebase/installations")
	app.Post("/c2dm/register3", game.AndroidRegister3).Name("android.apis.google.com/c2dm/register3")
	app.Get(`/:db/:filename<regex([a-z0-9]{64})>`, game.GetMaster).Name("climaster.game-idolypride.jp")
	app.Get("/v2/pub/a/212/v/:someid/list/:fromv", game.GetManifest).Name("api.octo-cloud.com")
	app.Get(`/solis-:revision<regex(\d{6})>-:type<alpha>/:object`, game.GetAsset).Name("d2ilil7yh5oi1v.cloudfront.net/solis")
	app.Post(`/v0/b/udc-idolypride/o`, game.PostPhoto).Name("firebasestorage.googleapis.com/v0/b/udc-idolypride/o")
	app.Get(`/v0/b/udc-idolypride/o/*`, game.GetPhoto).Name("firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo")

	// admin
	app.Post("/admin/user/create", admin.CreateUser)
	app.Post("/admin/user/delete", admin.DeleteUser)
	app.Post("/admin/user/god", admin.GodUser)
	app.Post("/admin/master/import", admin.ImportMaster)
	app.Get("/admin/master/status", admin.MasterImportStatus)

	// catch all for debug purpose
	app.All("*", func(c fiber.Ctx) error {
		return fiber.NewError(404, "no HTTP handler matches the request")
	})

	port := config.GetHttpPort()
	base.Infof("http server listening at port: %v", port)
	if err := app.Listen(":"+port, fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx,
	}); err != nil && ctx.Err() == nil {
		return fmt.Errorf("failed to serve HTTP: %w", err)
	}
	return nil
}
