package test

import (
	"fmt"
	"testing"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/http/game"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMigrateScenario(t *testing.T) {
	setupTestUser(t)

	mClient := papi.NewMigrationClient(testConfig.GrpcConn)

	// MigratePassword
	ctx1, cancel1 := getBasicGrpcCtx()
	defer cancel1()
	resp1, err := mClient.MigratePassword(ctx1, &papi.MigrationMigratePasswordRequest{
		UserId:   testConfig.TestUserPublicId,
		Password: testConfig.TestUserPass,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Logf("MigratePasswordResponse:\n%v", resp1)
	oneTimeToken := resp1.OnetimeToken

	// MigrationExecute
	ctx2, cancel2 := getBasicGrpcCtx()
	defer cancel2()
	resp2, err := mClient.Execute(ctx2, &papi.MigrationExecuteRequest{
		OnetimeToken: oneTimeToken,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Logf("MigrationExecuteResponse:\n%v", resp2)
	migrationToken := resp2.Token

	// one-time tokens must not be replayable
	replayCtx, replayCancel := getBasicGrpcCtx()
	defer replayCancel()
	if _, err := mClient.Execute(replayCtx, &papi.MigrationExecuteRequest{OnetimeToken: oneTimeToken}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("replayed one-time token returned %v, want %v", err, codes.Unauthenticated)
	}

	// verifyCustomToken
	resp3, err := testConfig.HttpClient.Post(fmt.Sprintf("%s/identitytoolkit/v3/relyingparty/verifyCustomToken", testConfig.HttpUrl), client.Config{
		Param: map[string]string{
			"key": testConfig.FirebaseProductKey,
		},
		Header: map[string]string{
			"X-Android-Package": "game.qualiarts.idolypride",
		},
		Body: &game.VerifyCustomTokenRequest{
			Token:             migrationToken,
			ReturnSecureToken: true,
		},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if resp3.StatusCode() != 200 {
		t.Fatalf("verifyCustomToken bad status code: %d", resp3.StatusCode())
	}
	var resp3Instance game.VerifyCustomTokenResponse
	if err := json.Unmarshal(resp3.Body(), &resp3Instance); err != nil {
		t.Fatalf("%v", err)
	}
	t.Logf("verifyCustomToken response:\n%v", resp3Instance)
	idToken := resp3Instance.IdToken
	refreshToken := resp3Instance.RefreshToken

	// getAccountInfo
	resp4, err := testConfig.HttpClient.Post(fmt.Sprintf("%s/identitytoolkit/v3/relyingparty/getAccountInfo", testConfig.HttpUrl), client.Config{
		Param: map[string]string{
			"key": testConfig.FirebaseProductKey,
		},
		Header: map[string]string{
			"X-Android-Package": "game.qualiarts.idolypride",
		},
		Body: &game.GetAccountInfoRequest{
			IdToken: idToken,
		},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if resp4.StatusCode() != 200 {
		t.Fatalf("getAccountInfo bad status code: %d", resp4.StatusCode())
	}
	var resp4Instance game.GetAccountInfoResponse
	if err := json.Unmarshal(resp4.Body(), &resp4Instance); err != nil {
		t.Fatalf("%v", err)
	}
	t.Logf("getAccountInfo response:\n%v", resp4Instance)

	aClient := papi.NewAuthClient(testConfig.GrpcConn)
	// authLogin
	ctx5, cancel5 := getBasicGrpcCtx()
	defer cancel5()
	resp5, err := aClient.Login(ctx5, &papi.AuthLoginRequest{
		FirebaseIDToken: idToken,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Logf("AuthLoginResponse: \n%v", resp5)

	if resp5.RequiredFirebaseReauthenticate {
		// /token
		resp6, err := testConfig.HttpClient.Post(fmt.Sprintf("%s/v1/token", testConfig.HttpUrl), client.Config{
			Param: map[string]string{
				"key": testConfig.FirebaseProductKey,
			},
			Header: map[string]string{
				"X-Android-Package": "game.qualiarts.idolypride",
			},
			Body: &game.TokenRequest{
				GrantType:    "refresh_token",
				RefreshToken: refreshToken,
			},
		})
		if err != nil {
			t.Fatalf("%v", err)
		}
		if resp6.StatusCode() != 200 {
			t.Fatalf("get secure token bad status code: %d", resp6.StatusCode())
		}
		var resp6Instance game.TokenResponse
		if err := json.Unmarshal(resp6.Body(), &resp6Instance); err != nil {
			t.Fatalf("%v", err)
		}
	}

}
