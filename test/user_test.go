package test

import (
	"fmt"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/http/admin"
	"testing"

	"github.com/gofiber/fiber/v3/client"
)

func TestUser(t *testing.T) {
	// delete existing user if there is
	testConfig.HttpClient.Post(fmt.Sprintf("%s/admin/user/delete", testConfig.HttpUrl), client.Config{
		Header: getAdminAuthHeader(),
		Body: &admin.UserDeleteRequest{
			ServerUserId: testConfig.TestUserSuid,
		},
	})

	// send user create request
	resp, err := testConfig.HttpClient.Post(fmt.Sprintf("%s/admin/user/create", testConfig.HttpUrl), client.Config{
		Header: getAdminAuthHeader(),
		Body: &admin.UserCreateRequest{
			PublicUserId: testConfig.TestUserPublicId,
			ServerUserId: testConfig.TestUserSuid,
			Password:     testConfig.TestUserPass,
		},
	})
	if err != nil {
		t.Fatalf("test create user failed: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("test create user error, bad status code: %d", resp.StatusCode())
	}

	// test `/api.User/Get`
	{
		uClient := papi.NewUserClient(testConfig.GrpcConn)
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatalf("%v", err)
		}
		defer cancel()
		resp, err := uClient.Get(ctx, &papi.Empty{})
		if err != nil {
			t.Fatalf("%v", err)
		}
		t.Logf("UserGetResponse:\n%v", resp)
	}

	// test `/api.Home/Login`
	{
		hClient := papi.NewHomeClient(testConfig.GrpcConn)
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatalf("%v", err)
		}
		defer cancel()
		resp, err := hClient.Login(ctx, &papi.HomeLoginRequest{})
		if err != nil {
			t.Fatalf("%v", err)
		}
		t.Logf("HomeLoginResponse:\n%v", resp)
	}

	// do cleanups, send user delete request
	resp2, err := testConfig.HttpClient.Post(fmt.Sprintf("%s/admin/user/delete", testConfig.HttpUrl), client.Config{
		Header: getAdminAuthHeader(),
		Body: &admin.UserDeleteRequest{
			ServerUserId: testConfig.TestUserSuid,
		},
	})
	if err != nil {
		t.Fatalf("test deleting user failed: %v", err)
	}
	if resp2.StatusCode() != 200 {
		t.Fatalf("test deleting user error, bad status code: %d", resp2.StatusCode())
	}
}
