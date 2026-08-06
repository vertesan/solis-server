package game

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/server/http/internal/diver"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"

	"fmt"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

type VerifyCustomTokenRequest struct {
	Token             string `json:"token"`
	ReturnSecureToken bool   `json:"returnSecureToken"`
}

type VerifyCustomTokenResponse struct {
	Kind         string `json:"kind"`
	IdToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    string `json:"expiresIn"`
	IsNewUser    bool   `json:"isNewUser"`
}

var FirebaseVerifyCustomToken fiber.Handler = func(c fiber.Ctx) error {
	logger := mcontext.GetLogger(c)
	key := c.Query("key")
	if key == "" {
		return diver.NewFiberError(400, "missing key query")
	}
	if key != depot.ServerProfile.FirebaseProductKey {
		return diver.NewFiberError(400, "invalid firebase product key")
	}

	verifyRequest := &VerifyCustomTokenRequest{}
	if err := json.Unmarshal(c.Body(), verifyRequest); err != nil {
		logger.Error("failed to unmarshal verifyRequest body", zap.Error(err))
		return fiber.NewError(400, "bad request body")
	}

	_, claim, err := jwto.ParseRS256Token(verifyRequest.Token)
	if err != nil {
		logger.Error("failed parsing token", zap.Error(err))
		return fiber.NewError(401, "invalid token")
	}

	uidObj, ok := (*claim)["uid"]
	if !ok {
		return diver.NewFiberError(400, "missing uid payload")
	}
	uid, ok := uidObj.(string)
	if !ok {
		return diver.NewFiberError(400, "invalid uid")
	}

	// retrieve userAuth from db
	userAuth, err := opuser.GetUserAuthByFirebaseId(context.Background(), uid)
	if err != nil {
		logger.Error("failed to GetUserAuthByFirebaseId", zap.Error(err))
		return fiber.NewError(403, "invalid user")
	}

	// create idToken
	idToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
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
		logger.Error("failed creating id token", zap.Error(err))
		return fiber.NewError(500, "internal error")
	}

	// check last refresh time, or rotate a new refresh token
	// TODO: revoke 7 days refreshing
	var refreshToken string
	if len(userAuth.RefreshTokens) == 0 || userAuth.LastRefreshAt.Add(time.Hour*24*7).Before(time.Now()) {
		refreshToken = jwto.CreateSecureRefreshToken()
		if err := opuser.InsertUserAuthRefreshToken(context.Background(), userAuth.FirebaseId, refreshToken); err != nil {
			logger.Error("failed to InsertRefreshToken", zap.Error(err))
			return fiber.NewError(500, "internal error")
		}
	} else {
		refreshToken = userAuth.RefreshTokens[len(userAuth.RefreshTokens)-1]
	}

	// set response contents
	c.Append("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC).Format("Mon, 02 Jan 2006 15:04:05 GMT"))
	c.Vary("Origin", "X-Origin", "Referer")
	return c.JSON(&VerifyCustomTokenResponse{
		Kind:         "identitytoolkit#VerifyCustomTokenResponse",
		IdToken:      idTokenString,
		RefreshToken: refreshToken,
		ExpiresIn:    "3600",
		IsNewUser:    true,
	})
}

type GetAccountInfoRequest struct {
	IdToken string `json:"idToken"`
}

type GetAccountInfoResponse struct {
	Kind  string `json:"kind"`
	Users []User `json:"users"`
}

type User struct {
	LocalId       string `json:"localId"`
	ValidSince    string `json:"validSince"`
	LastLoginAt   string `json:"lastLoginAt"`
	CreatedAt     string `json:"createdAt"`
	CustomAuth    bool   `json:"customAuth"`
	LastRefreshAt string `json:"lastRefreshAt"`
}

var FirebaseGetAccountInfo fiber.Handler = func(c fiber.Ctx) error {
	logger := mcontext.GetLogger(c)
	key := c.Query("key")
	if key == "" {
		return diver.NewFiberError(400, "missing key query")
	}
	if key != depot.ServerProfile.FirebaseProductKey {
		return diver.NewFiberError(400, "invalid firebase product key")
	}

	infoRequest := &GetAccountInfoRequest{}
	if err := json.Unmarshal(c.Body(), infoRequest); err != nil {
		logger.Error("failed to unmarshal infoRequest body", zap.Error(err))
		return fiber.NewError(400, "bad request body")
	}

	_, claim, err := jwto.ParseRS256Token(infoRequest.IdToken)
	if err != nil {
		logger.Error("failed parsing token", zap.Error(err))
		return fiber.NewError(401, "invalid token")
	}

	// check payload
	aud, ok := (*claim)["aud"]
	if !ok {
		return diver.NewFiberError(401, "missing aud payload")
	}
	if aud != "solis-firebase" {
		return diver.NewFiberError(403, "invalid payload")
	}

	// get firebase user id
	userIdObj, ok := (*claim)["user_id"]
	if !ok {
		return diver.NewFiberError(401, "missing user_id payload")
	}
	userId, ok := userIdObj.(string)
	if !ok {
		return diver.NewFiberError(403, "invalid payload")
	}

	// get user info from db
	userAuth, err := opuser.GetUserAuthByFirebaseId(context.Background(), userId)
	if err != nil {
		logger.Error("failed to GetUserAuthByFirebaseId", zap.Error(err))
		return fiber.NewError(500, "internal error")
	}

	// set response contents
	c.Append("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC).Format("Mon, 02 Jan 2006 15:04:05 GMT"))
	c.Vary("Origin", "X-Origin", "Referer")
	c.Set("X-Frame-Options", "SAMEORIGIN")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.JSON(&GetAccountInfoResponse{
		Kind: "identitytoolkit#GetAccountInfoResponse",
		Users: []User{
			{
				LocalId:       userId,
				ValidSince:    fmt.Sprint(userAuth.ValidSince),
				LastLoginAt:   fmt.Sprint(userAuth.LastLoginAt),
				CreatedAt:     fmt.Sprint(userAuth.CreatedAt),
				CustomAuth:    true,
				LastRefreshAt: userAuth.LastRefreshAt.Format("2006-01-02T15:04:05.000Z"),
			},
		},
	})
}
