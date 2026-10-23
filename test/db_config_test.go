package test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestDBConnectionConfiguration(t *testing.T) {
	// The retired URI must have no effect, including when credentials contain
	// URI delimiters or characters that need escaping.
	t.Setenv("SOLIS_DB_URI", "mongodb://wrong:wrong@wrong.example:1/")
	for _, test := range []struct {
		name, host, port, wantAddress string
	}{
		{name: "defaults", wantAddress: "localhost:27017"},
		{name: "compose host", host: "mongodb", port: "27018", wantAddress: "mongodb:27018"},
		{name: "IPv4", host: "127.0.0.1", wantAddress: "127.0.0.1:27017"},
		{name: "IPv6", host: "::1", wantAddress: "[::1]:27017"},
		{name: "bracketed IPv6", host: "[2001:db8::1]", port: "65535", wantAddress: "[2001:db8::1]:65535"},
	} {
		t.Run(test.name, func(t *testing.T) {
			const username = "manager:admin@日本"
			const password = "secret $:/?#[]@%+ &日本"
			t.Setenv("SOLIS_DB_USERNAME", username)
			t.Setenv("SOLIS_DB_PASSWORD", password)
			t.Setenv("SOLIS_DB_HOST", test.host)
			t.Setenv("SOLIS_DB_PORT", test.port)
			uri, err := config.GetDBString()
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(uri)
			if err != nil {
				t.Fatal("generated URI cannot be parsed")
			}
			decodedPassword, present := parsed.User.Password()
			if parsed.Scheme != "mongodb" || parsed.Host != test.wantAddress || parsed.User.Username() != username || !present || decodedPassword != password {
				t.Fatal("generated URI did not preserve connection settings and escaped credentials")
			}
			settings := options.Client().ApplyURI(uri)
			if err := settings.Validate(); err != nil {
				t.Fatal("generated URI was rejected by the MongoDB driver")
			}
			if settings.Auth == nil || settings.Auth.Username != username || settings.Auth.Password != password || settings.Auth.AuthSource != "admin" || settings.ReplicaSet == nil || *settings.ReplicaSet != "rs0" || settings.Direct == nil || !*settings.Direct {
				t.Fatal("MongoDB driver did not preserve credentials and replica-set configuration")
			}
		})
	}
	for _, test := range []struct{ name, key, value string }{
		{"missing username", "SOLIS_DB_USERNAME", ""},
		{"missing password", "SOLIS_DB_PASSWORD", ""},
		{"URI as host", "SOLIS_DB_HOST", "mongodb://localhost"},
		{"credentials as host", "SOLIS_DB_HOST", "user@localhost"},
		{"port as host", "SOLIS_DB_HOST", "localhost:27017"},
		{"query as host", "SOLIS_DB_HOST", "localhost?authSource=other"},
		{"path as host", "SOLIS_DB_HOST", "localhost/admin"},
		{"multiple hosts", "SOLIS_DB_HOST", "localhost,mongodb"},
		{"malformed IPv6", "SOLIS_DB_HOST", "[::invalid]"},
		{"malformed hostname", "SOLIS_DB_HOST", "bad..example"},
		{"host with spaces", "SOLIS_DB_HOST", " local host "},
		{"non-numeric port", "SOLIS_DB_PORT", "27017?replicaSet=other"},
		{"zero port", "SOLIS_DB_PORT", "0"},
		{"negative port", "SOLIS_DB_PORT", "-1"},
		{"out-of-range port", "SOLIS_DB_PORT", "65536"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("SOLIS_DB_USERNAME", "test-username")
			t.Setenv("SOLIS_DB_PASSWORD", "do-not-log-this")
			t.Setenv("SOLIS_DB_HOST", "localhost")
			t.Setenv("SOLIS_DB_PORT", "27017")
			t.Setenv(test.key, test.value)
			uri, err := config.GetDBString()
			if err == nil || uri != "" {
				t.Fatal("invalid configuration must not produce a connection URI")
			}
			if strings.Contains(err.Error(), "do-not-log-this") || strings.Contains(err.Error(), "test-username") {
				t.Fatal("configuration error disclosed credentials")
			}
		})
	}
}
