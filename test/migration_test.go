package test

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/http/game"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3/client"
)

func TestMigrateScenario(t *testing.T) {
	// remove previous test data, ignore error (not exists)
	opuser.DeleteUserAuth(context.Background(), "00000000-0000-0000-0000-000000000000")
	opuser.DeleteUser(context.Background(), "00000000-0000-0000-0000-000000000000")

	// prepare mock data
	now := time.Now()
	milliNow := now.UnixMilli()
	if err := opuser.InsertUserAuth(context.Background(), &umodel.UserAuth{
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
	if err := opuser.UpsertUser(context.Background(), "00000000-0000-0000-0000-000000000000", &ptransaction.User{
		Name:         "TestUser",
		ManagerLevel: 1,
	}); err != nil {
		t.Fatalf("failed to create mock user: %v", err)
	}

	mClient := papi.NewMigrationClient(testConfig.GrpcConn)

	// MigratePassword
	ctx1, cancel1 := getBasicGrpcCtx()
	defer cancel1()
	resp1, err := mClient.MigratePassword(ctx1, &papi.MigrationMigratePasswordRequest{
		UserId:   "00000000",
		Password: "password",
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

	// cleanup test mock data
	if err := opuser.DeleteUserAuth(context.Background(), "00000000-0000-0000-0000-000000000000"); err != nil {
		t.Fatalf("failed to remove test userAuth: %v", err)
	}
	if err := opuser.DeleteUser(context.Background(), "00000000-0000-0000-0000-000000000000"); err != nil {
		t.Fatalf("failed to remove test user: %v", err)
	}
}
