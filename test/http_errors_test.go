package test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/server/depot"
	httpserver "github.com/vertesan/solis-server/internal/server/http"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// Exercise the actual HTTP listener so errors returned before route middleware,
// including the body-size limit, follow the same policy as handler errors.
func TestHTTPErrorHandlingScenario(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_, port, err := net.SplitHostPort(address)
	listener.Close()
	if err != nil {
		t.Fatal(err)
	}
	const adminSecret = "http-error-test-admin-secret"
	const querySecret = "http-error-test-query-secret"
	const bodySecret = "19876543210987654321098765432109876543210"
	t.Setenv("SOLIS_HTTP_PORT", port)
	t.Setenv("SOLIS_ADMIN_AUTH", adminSecret)
	t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
	previousProfile := depot.ServerProfile
	depot.ServerProfile = &smodel.ServerProfile{PackageName: "http.error.test"}
	t.Cleanup(func() { depot.ServerProfile = previousProfile })

	core, observed := observer.New(zap.DebugLevel)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- httpserver.StartHTTP(ctx, zap.New(core).Sugar()) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("test HTTP server failed: %v", err)
			}
		case <-time.After(7 * time.Second):
			t.Error("test HTTP server did not stop")
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			connection.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("test HTTP server did not start: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	transport := &http.Transport{ExpectContinueTimeout: time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	baseURL := "http://" + address

	cases := []struct {
		name, method, path, body, authorization string
		details                                 []string
		status                                  int
		missingAdminConfiguration               bool
	}{
		{
			name: "early authentication", method: http.MethodPost,
			path:          "/admin/user/create?key=" + querySecret,
			authorization: "Bearer incorrect-" + adminSecret,
			status:        http.StatusUnauthorized, details: []string{"authorization"},
		},
		{
			name: "missing admin configuration", method: http.MethodPost,
			path: "/admin/user/create", authorization: "Bearer " + adminSecret,
			status: http.StatusInternalServerError, details: []string{"SOLIS_ADMIN_AUTH"},
			missingAdminConfiguration: true,
		},
		{
			name: "malformed JSON", method: http.MethodPost,
			path: "/admin/user/create", body: "{", authorization: "Bearer " + adminSecret,
			status: http.StatusBadRequest, details: []string{"request", "JSON"},
		},
		{
			name: "wrong JSON type without leaking value", method: http.MethodPost,
			path: "/admin/user/create", body: `{"password":` + bodySecret + `}`, authorization: "Bearer " + adminSecret,
			status: http.StatusBadRequest, details: []string{`field "password"`, "at byte", "expected string"},
		},
		{
			name: "validation", method: http.MethodGet,
			path: "/admin/master/status?id=not-a-job", authorization: "Bearer " + adminSecret,
			status: http.StatusBadRequest, details: []string{"job ID"},
		},
		{
			name: "game validation", method: http.MethodPost, path: "/c2dm/register3",
			status: http.StatusBadRequest, details: []string{"missing app header"},
		},
		{
			name: "filesystem failure", method: http.MethodGet,
			path:   "/master/" + strings.Repeat("a", 64),
			status: http.StatusInternalServerError, details: []string{"master"},
		},
		{
			name: "unmatched route", method: http.MethodGet,
			path:   "/not-a-real-route?key=" + querySecret,
			status: http.StatusNotFound, details: []string{"no HTTP handler"},
		},
		{
			name: "body-size limit", method: http.MethodPost, path: "/admin/user/create",
			body: strings.Repeat("x", 4*1024*1024+1), authorization: "Bearer " + adminSecret,
			status: http.StatusRequestEntityTooLarge, details: []string{"large"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.missingAdminConfiguration {
				t.Setenv("SOLIS_ADMIN_AUTH", "")
			}
			observed.TakeAll()
			request, err := http.NewRequest(tc.method, baseURL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", tc.authorization)
			if tc.status == http.StatusRequestEntityTooLarge {
				request.Header.Set("Expect", "100-continue")
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			wantBody := strings.ToLower(http.StatusText(tc.status))
			if tc.status >= 500 {
				wantBody = "internal error"
			}
			if response.StatusCode != tc.status || string(body) != wantBody {
				t.Fatalf("got HTTP %d %q, want %d %q", response.StatusCode, body, tc.status, wantBody)
			}
			if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/plain") {
				t.Fatalf("unexpected error content type: %q", response.Header.Get("Content-Type"))
			}
			var failures []observer.LoggedEntry
			for _, entry := range observed.TakeAll() {
				fields := entry.ContextMap()
				for _, secret := range []string{adminSecret, querySecret, bodySecret} {
					if strings.Contains(fmt.Sprint(fields), secret) || strings.Contains(entry.Message, secret) {
						t.Fatal("HTTP logs disclosed authorization, query, or request body contents")
					}
				}
				if _, ok := fields["status"]; ok {
					failures = append(failures, entry)
				}
			}
			if len(failures) != 1 {
				t.Fatalf("got %d error logs, want exactly one", len(failures))
			}
			fields := failures[0].ContextMap()
			if fields["status"] != int64(tc.status) || fields["method"] != tc.method || fields["api"] != request.URL.Path {
				t.Fatalf("incorrect HTTP error context: %v", fields)
			}
			for _, detail := range tc.details {
				if !strings.Contains(strings.ToLower(fmt.Sprint(fields["error"])), strings.ToLower(detail)) {
					t.Fatalf("original diagnostic missing from HTTP log: %v", fields)
				}
			}
		})
	}

	t.Run("maintenance", func(t *testing.T) {
		pauseCtx, pauseCancel := context.WithTimeout(context.Background(), time.Second)
		defer pauseCancel()
		defer masterdata.Default.Gate.Resume()
		if err := masterdata.Default.Gate.Pause(pauseCtx); err != nil {
			t.Fatal(err)
		}
		observed.TakeAll()
		request, err := http.NewRequest(http.MethodPost, baseURL+"/admin/user/create", strings.NewReader("{"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+adminSecret)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusServiceUnavailable || string(body) != "internal error" {
			t.Fatalf("maintenance returned HTTP %d %q", response.StatusCode, body)
		}
		entries := observed.FilterField(zap.Int("status", http.StatusServiceUnavailable)).All()
		if len(entries) != 1 || !strings.Contains(fmt.Sprint(entries[0].ContextMap()["error"]), "maintenance") {
			t.Fatalf("maintenance diagnostic was not logged once: %v", entries)
		}
	})

	t.Run("successful response unchanged", func(t *testing.T) {
		observed.TakeAll()
		request, err := http.NewRequest(http.MethodPost, baseURL+"/c2dm/register3", strings.NewReader("X-appid=http-error-test"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("app", depot.ServerProfile.PackageName)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || !strings.HasPrefix(string(body), "token=http-error-test:") {
			t.Fatalf("successful response changed: HTTP %d %q", response.StatusCode, body)
		}
		if observed.FilterLevelExact(zap.ErrorLevel).Len() != 0 {
			t.Fatal("successful request produced an error log")
		}
	})
}
