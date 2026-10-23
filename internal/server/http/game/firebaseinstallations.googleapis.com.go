package game

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/server/http/internal/diver"
	"github.com/vertesan/solis-server/internal/utils/jwto"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

type InstallationsRequest struct {
	Fid         string `json:"fid"`
	AppId       string `json:"appId"`
	AuthVersion string `json:"authVersion"`
	SdkVersion  string `json:"sdkVersion"`
}

type AuthToken struct {
	Token     string `json:"token"`
	ExpiresIn string `json:"expiresIn"`
}

type InstallationsReponse struct {
	Name         string    `json:"name"`
	Fid          string    `json:"fid"`
	RefreshToken string    `json:"refreshToken"`
	AuthToken    AuthToken `json:"authToken"`
}

var FirebaseInstallations fiber.Handler = func(c fiber.Ctx) error {
	// check header
	pkgId := c.Get("X-Android-Package")
	if pkgId == "" {
		return fiber.NewError(400, "missing X-Android-Package header")
	}
	if pkgId != depot.ServerProfile.PackageName {
		return fiber.NewError(400, "invalid X-Android-Package header")
	}

	// unmarshal request
	installationRequest := &InstallationsRequest{}
	if err := json.Unmarshal(c.Body(), installationRequest); err != nil {
		return fiber.NewError(400, fmt.Sprintf("decode Firebase installation request: %s", diver.JSONErrorDescription(err)))
	}

	// convert project number to int64
	appIds := strings.Split(installationRequest.AppId, ":")
	if len(appIds) < 2 {
		return fiber.NewError(400, "Firebase appId does not contain a project number")
	}
	projectNumber, err := strconv.ParseInt(appIds[1], 10, 64)
	if err != nil {
		return fiber.NewError(400, "Firebase appId project number is not a valid int64")
	}

	// create authToken
	authToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"appId":         installationRequest.AppId,
		"fid":           installationRequest.Fid,
		"projectNumber": projectNumber,
		"exp":           time.Now().Add(time.Hour * 24 * 7).Unix(),
	})
	authTokenString, err := jwto.CreateToken(authToken)
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("sign Firebase installation auth token: %v", err))
	}

	// create refresh token
	refreshToken := jwto.CreateInstallationRefreshToken()
	// TODO: save refreshToken to db?

	// set response contents
	c.Vary("Origin", "X-Origin", "Referer")
	return c.JSON(&InstallationsReponse{
		Name:         fmt.Sprintf("projects/%d/installations/%s", projectNumber, installationRequest.Fid),
		Fid:          installationRequest.Fid,
		RefreshToken: refreshToken,
		AuthToken: AuthToken{
			Token:     authTokenString,
			ExpiresIn: "604800s",
		},
	})
}
