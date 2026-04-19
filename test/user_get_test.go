package test

import (
	"fmt"
	"testing"
	"vertesan/solis-server/network/http/admin"
	"vertesan/solis-server/proto/papi"

	"github.com/gofiber/fiber/v3/client"
)

func TestUserGet(t *testing.T) {
	// delete existing user and ignore errors
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

	// get created user
	uClient := papi.NewUserClient(testConfig.GrpcConn)
	ctx1, cancel1, err := getFullGrpcCtx()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer cancel1()
	resp1, err := uClient.Get(ctx1, &papi.Empty{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Logf("UserGetResponse:\n%v", resp1)

	// send user delete request
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
