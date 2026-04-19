package test

import (
	"fmt"
	"testing"
	"vertesan/solis-server/network/http/admin"

	"github.com/gofiber/fiber/v3/client"
)

func TestUserGet(t *testing.T) {
	publicUserId := "00000000"
	serverUserId := "00000000-0000-0000-0000-000000000000"
	password := "password"

	// delete existing user and ignore errors
	testConfig.HttpClient.Post(fmt.Sprintf("%s/admin/user/delete", testConfig.HttpUrl), client.Config{
		Header: getAdminAuthHeader(),
		Body: &admin.UserDeleteRequest{
			ServerUserId: serverUserId,
		},
	})

	// send user create request
	resp, err := testConfig.HttpClient.Post(fmt.Sprintf("%s/admin/user/create", testConfig.HttpUrl), client.Config{
		Header: getAdminAuthHeader(),
		Body: &admin.UserCreateRequest{
			PublicUserId: publicUserId,
			ServerUserId: serverUserId,
			Password:     password,
		},
	})
	if err != nil {
		t.Fatalf("test create user failed: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("test create user error, bad status code: %d", resp.StatusCode())
	}

	// get created user
	


	// send user delete request
	resp2, err := testConfig.HttpClient.Post(fmt.Sprintf("%s/admin/user/delete", testConfig.HttpUrl), client.Config{
		Header: getAdminAuthHeader(),
		Body: &admin.UserDeleteRequest{
			ServerUserId: serverUserId,
		},
	})
	if err != nil {
		t.Fatalf("test deleting user failed: %v", err)
	}
	if resp2.StatusCode() != 200 {
		t.Fatalf("test deleting user error, bad status code: %d", resp2.StatusCode())
	}
}
