package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/command/makino"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/server/http/admin"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMakinoUserAdminScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	run := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		args = append([]string{"--server", testConfig.HttpUrl, "--timeout", "2m", "user"}, args...)
		if err := makino.Run(ctx, args, &stdout, &stderr); err != nil {
			t.Fatalf("management command failed: %v", err)
		}
		return stdout.Bytes()
	}

	// Exercise both supported identity forms without creating any non-configured user.
	run("delete", "--suid", testConfig.TestUserSuid)
	createData := run("create", "--id", testConfig.TestUserPublicId, "--suid", testConfig.TestUserSuid, "--password", testConfig.TestUserPass)
	var created admin.UserCreateResponse
	if err := json.Unmarshal(createData, &created); err != nil {
		t.Fatal(err)
	}
	if created.PublicUserId != testConfig.TestUserPublicId || created.ServerUserId != testConfig.TestUserSuid || created.Password != testConfig.TestUserPass {
		t.Fatal("create did not preserve the configured user credentials")
	}

	godData := run("god", "--id", testConfig.TestUserPublicId, "--message", "enjoy", "--fan", "none", "--bond", "none")
	var god admin.UserGodResponse
	if err := json.Unmarshal(godData, &god); err != nil {
		t.Fatal(err)
	}
	if !god.Ok || god.ServerUserId != testConfig.TestUserSuid || len(god.Collections) == 0 {
		t.Fatalf("unexpected god response: %+v", god)
	}
	run("delete", "--id", testConfig.TestUserPublicId)
	if _, err := opuser.GetUserAuthByServerId(ctx, testConfig.TestUserSuid); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("deleted test user still exists: %v", err)
	}
}

func TestMakinoUserRequestSafetyScenario(t *testing.T) {
	const token = "makino-test-admin-token"
	t.Setenv("SOLIS_ADMIN_AUTH", token)
	// User commands must not connect to MongoDB, even with an unusable DB setting.
	t.Setenv("SOLIS_DB_USERNAME", "")
	ctx := context.Background()
	var got admin.UserGodRequest
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/admin/user/god" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("incorrect admin request method, path, or authorization")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)
	t.Setenv("MAKINO_SERVER_URL", server.URL)
	if err := makino.Run(ctx, []string{"user", "god", "--id", testConfig.TestUserPublicId, "--message", "enjoy", "--fan", "none", "--bond", "all"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got.PublicUserId != testConfig.TestUserPublicId || got.ServerUserId != "" || got.Feature != (admin.GodFeature{Message: "enjoy", Fan: "none", Bond: "all"}) {
		t.Fatalf("god command did not forward identity/features: %+v", got)
	}
	for _, args := range [][]string{
		{"user", "delete"},
		{"user", "delete", "--id", testConfig.TestUserPublicId, "--suid", testConfig.TestUserSuid},
		{"user", "god", "--id", testConfig.TestUserPublicId, "--message", "invalid"},
		{"user", "god", "--id", testConfig.TestUserPublicId, "unexpected"},
		{"--server", "http://user:secret@localhost", "user", "god", "--id", testConfig.TestUserPublicId},
		{"--timeout", "0s", "user", "god", "--id", testConfig.TestUserPublicId},
	} {
		if err := makino.Run(ctx, args, io.Discard, io.Discard); err == nil {
			t.Fatalf("invalid command was accepted: %v", args)
		}
	}
	if calls != 1 {
		t.Fatalf("invalid commands reached admin API: %d calls", calls)
	}

	redirectCalls := 0
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCalls++
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(redirectTarget.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", redirectTarget.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
		io.WriteString(w, token+" "+testConfig.TestUserPass)
	}))
	t.Cleanup(redirector.Close)
	var stdout bytes.Buffer
	err := makino.Run(ctx, []string{"--server", redirector.URL, "user", "create", "--id", testConfig.TestUserPublicId, "--suid", testConfig.TestUserSuid, "--password", testConfig.TestUserPass}, &stdout, io.Discard)
	if err == nil || redirectCalls != 0 || stdout.Len() != 0 {
		t.Fatalf("redirect was not safely rejected: err=%v calls=%d", err, redirectCalls)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), testConfig.TestUserPass) {
		t.Fatal("admin failure exposed credentials")
	}
}
