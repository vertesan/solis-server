package utils

import (
	"bufio"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/vertesan/solis-server/internal/utils/rich"
	"math/rand"
	"os"
	"time"

	"github.com/goccy/go-json"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func UNUSED(x ...interface{}) {}

func GetTimestamp() int64 {
	return time.Now().UnixMilli()
}

func RandomHexString(length int) string {
	if length <= 0 {
		rich.Fatal("length must be a positive integer")
	}

	// Each byte gives two hex characters, so we need (length+1)/2 bytes to cover odd lengths.
	numBytes := (length + 1) / 2
	bytes := make([]byte, numBytes)

	_, err := crand.Read(bytes)
	if err != nil {
		rich.Fatal("failed to read random bytes")
	}

	hexStr := hex.EncodeToString(bytes)

	// Trim the hex string to the exact requested length
	return hexStr[:length]
}

func RandomLengthString() ([]byte, int8) {
	length := rand.Intn(30) + 3
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(61)]
	}
	return result, int8(length)
}

func RandomString(length int) string {
	if length <= 0 {
		rich.Fatal("length must be a positive integer")
	}

	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(61)]
	}
	return string(result)
}

func ReadJsonFile[T any](path string) (*T, error) {
	obj := new(T)
	fs, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fs.Close()
	reader := bufio.NewReaderSize(fs, 1024*1024)
	if err := json.NewDecoder(reader).Decode(obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json file: %w", err)
	}
	return obj, nil
}
