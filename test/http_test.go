package test

import (
	"context"
	"fmt"
	"testing"
	"time"
	"vertesan/solis-server/db/crud/opuser"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/network/http/game"
	"vertesan/solis-server/utils/jwto"

	"github.com/gofiber/fiber/v3/client"
	"github.com/golang-jwt/jwt/v5"
)

func TestQseedGetId(t *testing.T) {
	resp, err := testConfig.HttpClient.Get(fmt.Sprintf("%s/getId", testConfig.HttpUrl), client.Config{
		Header: map[string]string{
			"X-App":    "game.qualiarts.idolypride",
			"X-Device": "1a6cc5752d4997b2",
		},
	})
	if err != nil {
		t.Fatalf("get qseed error: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("get qseed error, bad status code: %d", resp.StatusCode())
	}
	traceId := resp.Header("X-Cloud-Trace-Context")
	if len(traceId) == 0 {
		t.Error("no X-Cloud-Trace-Context in response header while get qseed id")
	}
	t.Logf("x-cloud-trace-context: %s", traceId)

	qseed := resp.String()
	if qseed == "" {
		t.Error("get empty qseed")
	}
	t.Logf("qseed: %s", qseed)
}

func TestVerifyCustomToken(t *testing.T) {
	// remove previous test data, ignore error (not exists)
	opuser.DeleteUserAuth(context.Background(), "00000000-0000-0000-0000-000000000000")

	// prepare mock data
	now := time.Now()
	milliNow := now.UnixMilli()
	if err := opuser.InsertUserAuth(context.Background(), &ustruct.UserAuth{
		PublicUserId:  "00000000",
		ServerUserId:  "00000000-0000-0000-0000-000000000000",
		Password:      "password",
		FirebaseId:    "00000000-0000-0000-0000-000000000000",
		RefreshTokens: []string{jwto.CreateSecureRefreshToken()},
		ValidSince:    now.Unix(),
		CreatedAt:     milliNow,
		LastLoginAt:   milliNow,
		LastRefreshAt: now,
		OnetimeToken:  "",
	}); err != nil {
		t.Fatalf("failed to create mock userAuth: %v", err)
	}

	token, err := jwto.CreateToken(jwt.NewWithClaims(jwt.SigningMethodRS256, &jwt.MapClaims{
		"iss": "firebase-adminsdk-hvn46@solis-firebase.iam.gserviceaccount.com",
		"aud": "https://identitytoolkit.googleapis.com/google.identity.identitytoolkit.v1.IdentityToolkit",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"sub": "firebase-adminsdk-hvn46@solis-firebase.iam.gserviceaccount.com",
		"uid": "00000000-0000-0000-0000-000000000000",
	}))
	if err != nil {
		t.Fatalf("failed creating token: %v", err)
	}

	resp, err := testConfig.HttpClient.Post(fmt.Sprintf("%s/identitytoolkit/v3/relyingparty/verifyCustomToken", testConfig.HttpUrl), client.Config{
		Param: map[string]string{
			"key": testConfig.FirebaseProductKey,
		},
		Header: map[string]string{
			"X-Android-Package": "game.qualiarts.idolypride",
		},
		Body: &game.VerifyCustomTokenRequest{
			Token:             token,
			ReturnSecureToken: true,
		},
	})
	if err != nil {
		t.Fatalf("get qseed error: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("get qseed error, bad status code: %d", resp.StatusCode())
	}
	t.Logf("resp: %v", resp)

	// cleanup test mock data
	if err := opuser.DeleteUserAuth(context.Background(), "00000000-0000-0000-0000-000000000000"); err != nil {
		t.Fatalf("failed to remove test userAuth: %v", err)
	}
}
