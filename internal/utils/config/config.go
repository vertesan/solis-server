package config

import (
	"fmt"
	"os"
	"path"
	"strconv"
)

func getOrDefault(key string, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

func GetLogType() string {
	return getOrDefault("SOLIS_LOG_TYPE", "2")
}

func GetEnv() string {
	return getOrDefault("SOLIS_ENV", "dev")
}

func GetGrpcPort() string {
	return getOrDefault("SOLIS_GRPC_PORT", "50051")
}

func GetHttpPort() string {
	return getOrDefault("SOLIS_HTTP_PORT", "50052")
}

func GetDBString() (string, error) {
	conn := os.Getenv("SOLIS_DB_URI")
	if conn == "" {
		return "", fmt.Errorf("SOLIS_DB_URI does not exist")
	}
	return conn, nil
}

func GetResourceDir() string {
	return getOrDefault("SOLIS_RESOURCE_DIR", "/app/resources")
}

func GetDBDir() string {
	return path.Join(GetResourceDir(), "db")
}

func GetAssetDir() string {
	return path.Join(GetResourceDir(), "assets")
}

func GetManifestDir() string {
	return path.Join(GetResourceDir(), "manifest")
}

func GetAdminAuth() (string, error) {
	auth := os.Getenv("SOLIS_ADMIN_AUTH")
	if auth == "" {
		return "", fmt.Errorf("SOLIS_ADMIN_AUTH does not exist")
	}
	return auth, nil
}

func GetGameSettingId() (int64, error) {
	id, err := strconv.ParseInt(getOrDefault("SOLIS_SETTING_ID", "1"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("SOLIS_SETTING_ID must be a valid integer: %w", err)
	}
	return id, nil
}

func GetRSAPrivateKey() ([]byte, error) {
	filepath := os.Getenv("SOLIS_JWT_RSA_PRIV_KEY_PATH")
	if filepath == "" {
		return nil, fmt.Errorf("SOLIS_JWT_RSA_PRIV_KEY_PATH does not exist")
	}
	keyData, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed reading RSA private key from %q: %w", filepath, err)
	}

	// block, _ := pem.Decode(keyData)
	// if block == nil || (block.Type != "PRIVATE KEY" && block.Type != "RSA PRIVATE KEY") {
	// 	return nil, fmt.Errorf("failed decode RSA private key: %v", block)
	// }

	// privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	// if err != nil {
	// 	return nil, fmt.Errorf("unable to parse RSA private key: %w", err)
	// }
	// return privateKey
	return keyData, nil
}

func GetRSAPublicKey() ([]byte, error) {
	filepath := os.Getenv("SOLIS_JWT_RSA_PUB_KEY_PATH")
	if filepath == "" {
		return nil, fmt.Errorf("SOLIS_JWT_RSA_PUB_KEY_PATH does not exist")
	}
	keyData, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed reading RSA public key from %q: %w", filepath, err)
	}

	// block, _ := pem.Decode(keyData)
	// if block == nil || block.Type != "PUBLIC KEY" {
	// 	return nil, fmt.Errorf("failed to decode RSA public key: %v", block)
	// }

	// pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	// if err != nil {
	// 	return nil, fmt.Errorf("unable to parse RSA public key: %w", err)
	// }
	// pubKey, ok := pubInterface.(*rsa.PublicKey)
	// if !ok {
	// 	return nil, fmt.Errorf("parsed key is not a valid RSA public key: %v", pubInterface)
	// }
	// return pubKey
	return keyData, nil
}

func GetHMACKey() ([]byte, error) {
	key := os.Getenv("SOLIS_JWT_HMAC_KEY")
	if key == "" {
		return nil, fmt.Errorf("SOLIS_JWT_HMAC_KEY does not exist")
	}
	return []byte(key), nil
}

func GetGrpcCertKeyPath() (string, error) {
	path := os.Getenv("SOLIS_GRPC_CERT_PRIV_KEY_PATH")
	if path == "" {
		return "", fmt.Errorf("SOLIS_GRPC_CERT_PRIV_KEY_PATH does not exist")
	}
	return path, nil
}

func GetGrpcCertPath() (string, error) {
	path := os.Getenv("SOLIS_GRPC_CERT_PUB_KEY_PATH")
	if path == "" {
		return "", fmt.Errorf("SOLIS_GRPC_CERT_PUB_KEY_PATH does not exist")
	}
	return path, nil
}
