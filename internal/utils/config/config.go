package config

import (
	"github.com/vertesan/solis-server/internal/utils/rich"
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

func GetEnv() string {
	return getOrDefault("SOLIS_ENV", "dev")
}

func GetGrpcPort() string {
	return getOrDefault("SOLIS_GRPC_PORT", "50051")
}

func GetHttpPort() string {
	return getOrDefault("SOLIS_HTTP_PORT", "50052")
}

func GetDBString() string {
	conn := os.Getenv("SOLIS_DB_URI")
	if conn == "" {
		rich.Fatal("DB connection string env SOLIS_DB_URI not exists")
	}
	return conn
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

func GetAdminAuth() string {
	auth := os.Getenv("SOLIS_ADMIN_AUTH")
	if auth == "" {
		rich.Fatal("SOLIS_ADMIN_AUTH does not exist")
	}
	return auth
}

func GetGameSettingId() int64 {
	id, err := strconv.ParseInt(getOrDefault("SOLIS_SETTING_ID", "1"), 10, 64)
	if err != nil {
		rich.Fatal("SOLIS_SETTING_ID must be a valid integer")
	}
	return id
}

func GetRSAPrivateKey() []byte {
	filepath := os.Getenv("SOLIS_RSA_PRIV_KEY")
	if filepath == "" {
		rich.Fatal("env SOLIS_RSA_PRIV_KEY does not exist")
	}
	keyData, err := os.ReadFile(filepath)
	if err != nil {
		rich.Fatal("failed reading RSA private key from %s: %v", filepath, err)
	}

	// block, _ := pem.Decode(keyData)
	// if block == nil || (block.Type != "PRIVATE KEY" && block.Type != "RSA PRIVATE KEY") {
	// 	rich.Fatal("failed decode RSA private key, block:\n%v", block)
	// }

	// privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	// if err != nil {
	// 	rich.Fatal("unable to parse RSA private key: %v", err)
	// }
	// return privateKey
	return keyData
}

func GetRSAPublicKey() []byte {
	filepath := os.Getenv("SOLIS_RSA_PUB_KEY")
	if filepath == "" {
		rich.Fatal("env SOLIS_RSA_PUB_KEY does not exist")
	}
	keyData, err := os.ReadFile(filepath)
	if err != nil {
		rich.Fatal("failed reading RSA public key from %s: %v", filepath, err)
	}

	// block, _ := pem.Decode(keyData)
	// if block == nil || block.Type != "PUBLIC KEY" {
	// 	rich.Fatal("failed to decode RSA public key, block:\n%v", block)
	// }

	// pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	// if err != nil {
	// 	rich.Fatal("unable to parse RSA public key: %v", err)
	// }
	// pubKey, ok := pubInterface.(*rsa.PublicKey)
	// if !ok {
	// 	rich.Fatal("parsed key is not a valid RSA public key: %v", pubInterface)
	// }
	// return pubKey
	return keyData
}

func GetHMACKey() []byte {
	key := os.Getenv("SOLIS_HMAC_KEY")
	if key == "" {
		rich.Fatal("env SOLIS_HMAC_KEY does not exist")
	}
	return []byte(key)
}

// Dev configs

func IsNoCodec() bool {
	return os.Getenv("SOLIS_DEV_NO_CODEC") == "1" && GetEnv() == "dev"
}

func IsNoCert() bool {
	return GetEnv() != "dev" || os.Getenv("SOLIS_DEV_NO_CERT") == "1"
}

func GetGrpcCertKeyPath() string {
	path := os.Getenv("SOLIS_DEV_GRPC_CERT_KEY_PATH")
	if path == "" {
		rich.Fatal("certificate key env not found")
	}
	return path
}

func GetGrpcCertPath() string {
	path := os.Getenv("SOLIS_DEV_GRPC_CERT_PATH")
	if path == "" {
		rich.Fatal("certificate env not found")
	}
	return path
}
