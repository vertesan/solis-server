package makino

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/vertesan/solis-server/internal/server/http/admin"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func runUser(ctx context.Context, serverURL string, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printUsage(stdout)
		return nil
	}
	command := args[0]
	if command != "create" && command != "god" && command != "delete" {
		return errors.New("unknown user command; expected create, god, or delete")
	}
	flags := flag.NewFlagSet("makino user "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	publicID := flags.String("id", "", "public user ID")
	serverID := flags.String("suid", "", "server user UUID")
	var password string
	var feature admin.GodFeature
	if command == "create" {
		flags.StringVar(&password, "password", "", "transfer password (server-generated if omitted)")
	}
	if command == "god" {
		flags.StringVar(&feature.Message, "message", "", "message and telephone policy: all, enjoy, or none")
		flags.StringVar(&feature.Fan, "fan", "", "fan policy: all, enjoy, or none")
		flags.StringVar(&feature.Bond, "bond", "", "reliability policy: all, enjoy, or none")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments after user command")
	}
	if command != "create" && ((*publicID == "") == (*serverID == "")) {
		return errors.New("exactly one of --id or --suid is required")
	}
	for name, value := range map[string]string{"message": feature.Message, "fan": feature.Fan, "bond": feature.Bond} {
		switch value {
		case "", "all", "enjoy", "none":
		default:
			return fmt.Errorf("--%s must be all, enjoy, or none", name)
		}
	}
	var body any
	switch command {
	case "create":
		body = admin.UserCreateRequest{PublicUserId: *publicID, ServerUserId: *serverID, Password: password}
	case "god":
		body = admin.UserGodRequest{PublicUserId: *publicID, ServerUserId: *serverID, Feature: feature}
	case "delete":
		body = admin.UserDeleteRequest{PublicUserId: *publicID, ServerUserId: *serverID}
	}
	return postAdmin(ctx, serverURL, "/admin/user/"+command, body, password, stdout)
}

func postAdmin(ctx context.Context, serverURL, path string, body any, password string, stdout io.Writer) error {
	baseURL, err := url.Parse(serverURL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") ||
		baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return errors.New("--server must be an HTTP(S) URL without credentials, a query, or a fragment")
	}
	token, err := config.GetAdminAuth()
	if err != nil {
		return err
	}
	redact := func(message string) string {
		for _, secret := range []string{token, password} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[redacted]")
			}
		}
		return message
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return errors.New("cannot encode admin request")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + path
	baseURL.RawPath = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL.String(), bytes.NewReader(payload))
	if err != nil {
		return errors.New("cannot build admin request")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		// Never forward administrator credentials or request bodies to a redirect.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("admin request failed: %s", redact(err.Error()))
	}
	defer response.Body.Close()
	const maxResponseSize = 1 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return fmt.Errorf("cannot read admin response: %s", redact(err.Error()))
	}
	if len(data) > maxResponseSize {
		return errors.New("admin response exceeds 1 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("admin API returned HTTP %d: %s", response.StatusCode, redact(strings.TrimSpace(string(data))))
	}
	if !json.Valid(data) {
		return errors.New("admin API returned invalid JSON")
	}
	_, err = fmt.Fprintln(stdout, string(data))
	return err
}
