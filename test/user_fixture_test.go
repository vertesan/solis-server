package test

import (
	"fmt"
	"testing"

	"github.com/vertesan/solis-server/internal/server/http/admin"

	"github.com/gofiber/fiber/v3/client"
)

func setupTestUser(t *testing.T) {
	t.Helper()

	if err := deleteConfiguredTestUser(); err != nil {
		t.Fatalf("failed to reset test user: %v", err)
	}
	t.Cleanup(func() {
		if err := deleteConfiguredTestUser(); err != nil {
			t.Errorf("failed to clean up test user: %v", err)
		}
	})

	response, err := testConfig.HttpClient.Post(
		fmt.Sprintf("%s/admin/user/create", testConfig.HttpUrl),
		client.Config{
			Header: getAdminAuthHeader(),
			Body: &admin.UserCreateRequest{
				PublicUserId: testConfig.TestUserPublicId,
				ServerUserId: testConfig.TestUserSuid,
				Password:     testConfig.TestUserPass,
			},
		},
	)
	if err != nil {
		t.Fatalf("test create user failed: %v", err)
	}
	if response.StatusCode() != 200 {
		t.Fatalf("test create user returned status %d: %s", response.StatusCode(), response.Body())
	}
}

func deleteConfiguredTestUser() error {
	response, err := testConfig.HttpClient.Post(
		fmt.Sprintf("%s/admin/user/delete", testConfig.HttpUrl),
		client.Config{
			Header: getAdminAuthHeader(),
			Body: &admin.UserDeleteRequest{
				ServerUserId: testConfig.TestUserSuid,
			},
		},
	)
	if err != nil {
		return err
	}
	if response.StatusCode() != 200 && response.StatusCode() != 400 {
		return fmt.Errorf("delete returned status %d: %s", response.StatusCode(), response.Body())
	}
	return nil
}
