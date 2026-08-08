package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestUserAuthTokenUniquenessConstraints verifies empty token sets can coexist
// while nonempty refresh and one-time tokens remain unique across accounts.
func TestUserAuthTokenUniquenessConstraints(t *testing.T) {
	ctx := context.Background()
	first := testUserAuth()
	second := testUserAuth()
	for _, auth := range []*umodel.UserAuth{first, second} {
		if err := opuser.InsertUserAuth(ctx, auth); err != nil {
			t.Fatalf("insert userAuth with an empty refresh-token array: %v", err)
		}
		auth := auth
		t.Cleanup(func() {
			if err := opuser.DeleteUserAuth(context.Background(), auth.ServerUserId); err != nil {
				t.Errorf("delete test userAuth %q: %v", auth.ServerUserId, err)
			}
		})
	}

	refreshToken := uuid.NewString()
	if err := opuser.InsertUserAuthRefreshToken(ctx, first.FirebaseId, refreshToken); err != nil {
		t.Fatalf("add first refresh token: %v", err)
	}
	if err := opuser.InsertUserAuthRefreshToken(ctx, second.FirebaseId, refreshToken); !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("duplicate refresh token returned %v, want duplicate-key error", err)
	}

	oneTimeToken := uuid.NewString()
	if err := opuser.UpdateUserAuthOneTimeToken(ctx, first.ServerUserId, oneTimeToken); err != nil {
		t.Fatalf("set first one-time token: %v", err)
	}
	if err := opuser.UpdateUserAuthOneTimeToken(ctx, second.ServerUserId, oneTimeToken); !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("duplicate one-time token returned %v, want duplicate-key error", err)
	}
}

// testUserAuth returns an isolated valid auth document for index integration
// tests.
func testUserAuth() *umodel.UserAuth {
	return &umodel.UserAuth{
		PublicUserId:  uuid.NewString(),
		ServerUserId:  uuid.NewString(),
		FirebaseId:    uuid.NewString(),
		RefreshTokens: []string{},
	}
}
