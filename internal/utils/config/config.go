package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
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
	username := os.Getenv("SOLIS_DB_USERNAME")
	password := os.Getenv("SOLIS_DB_PASSWORD")
	if username == "" || password == "" {
		return "", fmt.Errorf("SOLIS_DB_USERNAME and SOLIS_DB_PASSWORD are required")
	}
	host := getOrDefault("SOLIS_DB_HOST", "localhost")
	// Accept IPv6 with or without brackets, but never accept a URI, multiple
	// hosts, or a port here: those would change the assembled URI's meaning.
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
		if net.ParseIP(host) == nil {
			return "", fmt.Errorf("SOLIS_DB_HOST must be a hostname or IP address")
		}
	}
	if !validDBHost(host) {
		return "", fmt.Errorf("SOLIS_DB_HOST must be a hostname or IP address")
	}
	port, err := strconv.Atoi(getOrDefault("SOLIS_DB_PORT", "27017"))
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("SOLIS_DB_PORT must be an integer between 1 and 65535")
	}
	// Solis uses transactions on a single-node replica set, including when
	// connecting through Docker's published port instead of its internal name.
	uri := url.URL{
		Scheme: "mongodb",
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/",
		RawQuery: url.Values{
			"authSource":       {"admin"},
			"replicaSet":       {"rs0"},
			"directConnection": {"true"},
		}.Encode(),
	}
	return uri.String(), nil
}

func validDBHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
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
