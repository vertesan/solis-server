package test

import (
	"testing"

	"github.com/vertesan/solis-server/internal/utils/config"
)

func TestRequiredConfigReturnsError(t *testing.T) {
	tests := []struct {
		name string
		key  string
		get  func() error
	}{
		{name: "database username", key: "SOLIS_DB_USERNAME", get: func() error { _, err := config.GetDBString(); return err }},
		{name: "database password", key: "SOLIS_DB_PASSWORD", get: func() error { _, err := config.GetDBString(); return err }},
		{name: "admin auth", key: "SOLIS_ADMIN_AUTH", get: func() error { _, err := config.GetAdminAuth(); return err }},
		{name: "RSA private key", key: "SOLIS_JWT_RSA_PRIV_KEY_PATH", get: func() error { _, err := config.GetRSAPrivateKey(); return err }},
		{name: "RSA public key", key: "SOLIS_JWT_RSA_PUB_KEY_PATH", get: func() error { _, err := config.GetRSAPublicKey(); return err }},
		{name: "HMAC key", key: "SOLIS_JWT_HMAC_KEY", get: func() error { _, err := config.GetHMACKey(); return err }},
		{name: "gRPC certificate key", key: "SOLIS_GRPC_CERT_PRIV_KEY_PATH", get: func() error { _, err := config.GetGrpcCertKeyPath(); return err }},
		{name: "gRPC certificate", key: "SOLIS_GRPC_CERT_PUB_KEY_PATH", get: func() error { _, err := config.GetGrpcCertPath(); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.key, "")
			if err := test.get(); err == nil {
				t.Fatalf("expected an error when %s is missing", test.key)
			}
		})
	}
}

func TestGetGameSettingIdReturnsParseError(t *testing.T) {
	t.Setenv("SOLIS_SETTING_ID", "invalid")
	if _, err := config.GetGameSettingId(); err == nil {
		t.Fatal("expected invalid SOLIS_SETTING_ID to return an error")
	}
}
