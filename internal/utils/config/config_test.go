package config

import "testing"

func TestRequiredConfigReturnsError(t *testing.T) {
	tests := []struct {
		name string
		key  string
		get  func() error
	}{
		{name: "database URI", key: "SOLIS_DB_URI", get: func() error { _, err := GetDBString(); return err }},
		{name: "admin auth", key: "SOLIS_ADMIN_AUTH", get: func() error { _, err := GetAdminAuth(); return err }},
		{name: "RSA private key", key: "SOLIS_RSA_PRIV_KEY", get: func() error { _, err := GetRSAPrivateKey(); return err }},
		{name: "RSA public key", key: "SOLIS_RSA_PUB_KEY", get: func() error { _, err := GetRSAPublicKey(); return err }},
		{name: "HMAC key", key: "SOLIS_HMAC_KEY", get: func() error { _, err := GetHMACKey(); return err }},
		{name: "gRPC certificate key", key: "SOLIS_DEV_GRPC_CERT_KEY_PATH", get: func() error { _, err := GetGrpcCertKeyPath(); return err }},
		{name: "gRPC certificate", key: "SOLIS_DEV_GRPC_CERT_PATH", get: func() error { _, err := GetGrpcCertPath(); return err }},
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
	if _, err := GetGameSettingId(); err == nil {
		t.Fatal("expected invalid SOLIS_SETTING_ID to return an error")
	}
}
