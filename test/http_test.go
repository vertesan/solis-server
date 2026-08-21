package test

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/server/http/game"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"testing"
	"time"

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
	setupTestUser(t)
	userAuth, err := opuser.GetUserAuthByServerId(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatalf("failed to load configured test user auth: %v", err)
	}

	token, err := jwto.CreateToken(jwt.NewWithClaims(jwt.SigningMethodRS256, &jwt.MapClaims{
		"iss": "firebase-adminsdk-hvn46@solis-firebase.iam.gserviceaccount.com",
		"aud": "https://identitytoolkit.googleapis.com/google.identity.identitytoolkit.v1.IdentityToolkit",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"sub": "firebase-adminsdk-hvn46@solis-firebase.iam.gserviceaccount.com",
		"uid": userAuth.FirebaseId,
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
}
