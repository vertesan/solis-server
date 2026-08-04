package game

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/server/internal/depot"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"github.com/vertesan/solis-server/internal/utils/rich"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

type TokenRequest struct {
	GrantType    string `json:"grantType"`
	RefreshToken string `json:"refreshToken"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    string `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	IdToken      string `json:"id_token"`
	UserId       string `json:"user_id"`
	ProjectId    string `json:"project_id"`
}

var FirebaseToken fiber.Handler = func(c fiber.Ctx) error {
	key := c.Query("key")
	if key == "" {
		return fiber.NewError(400, "missing key query")
	}
	if key != depot.ServerProfile.FirebaseProductKey {
		return fiber.NewError(400, "invalid firebase product key")
	}

	// check header
	pkgId := c.Get("X-Android-Package")
	if pkgId == "" {
		return fiber.NewError(400, "missing X-Android-Package header")
	}
	if pkgId != depot.ServerProfile.PackageName {
		return fiber.NewError(400, "invalid X-Android-Package header")
	}

	tokenReq := &TokenRequest{}
	if err := json.Unmarshal(c.Body(), tokenReq); err != nil {
		return fiber.NewError(400, "bad request body")
	}

	if tokenReq.GrantType != "refresh_token" {
		return fiber.NewError(400, "invalid grant type")
	}

	// retrieve userAuth by refresh token
	userAuth, err := opuser.GetUserAuthByRefreshToken(context.Background(), tokenReq.RefreshToken)
	if err != nil {
		return fiber.NewError(500, "invernal error")
	}

	// check last refresh time, or rotate a new refresh token
	// TODO: revoke 7 days refreshing
	var refreshToken string
	if userAuth.LastRefreshAt.Add(time.Hour * 24 * 7).Before(time.Now()) {
		refreshToken = jwto.CreateSecureRefreshToken()
		if err := opuser.InsertUserAuthRefreshToken(context.Background(), userAuth.FirebaseId, refreshToken); err != nil {
			return fiber.NewError(500, "internal error")
		}
	} else {
		refreshToken = userAuth.RefreshTokens[len(userAuth.RefreshTokens)-1]
	}

	// create idToken
	idToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"puid":      userAuth.PublicUserId,
		"iss":       "https://securetoken.google.com/solis-firebase",
		"aud":       "solis-firebase",
		"auth_time": time.Now().Unix(),
		"user_id":   userAuth.FirebaseId,
		"sub":       userAuth.FirebaseId,
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
		"firebase": map[string]any{
			"identities":       map[string]any{},
			"sign_in_provider": "custom",
		},
	})
	idToken.Header["kid"] = utils.RandomHexString(40)
	idTokenString, err := jwto.CreateToken(idToken)
	if err != nil {
		rich.Error("failed creating id token: %v", err)
		return fiber.NewError(500, "internal error")
	}

	// set response contents
	c.Append("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC).Format("Mon, 02 Jan 2006 15:04:05 GMT"))
	c.Vary("Origin", "X-Origin", "Referer")
	return c.JSON(&TokenResponse{
		AccessToken:  idTokenString,
		ExpiresIn:    "3600",
		TokenType:    "Bearer",
		RefreshToken: refreshToken,
		IdToken:      idTokenString,
		UserId:       userAuth.FirebaseId,
		ProjectId:    depot.ServerProfile.FirebaseProjectId,
	})
}
