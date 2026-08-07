package crypto

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"io"

	"go.uber.org/zap"
)

func EncryptOctoManifest(reader io.Reader, key []byte) ([]byte, error) {
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		zap.L().Error("failed generating 16 random bytes iv", zap.Error(err))
		return nil, err
	}
	buf := &bytes.Buffer{}
	writer := bufio.NewWriter(buf)
	writer.Write(iv)
	if err := writer.Flush(); err != nil {
		zap.L().Error("failed flushing writer", zap.Error(err))
		return nil, err
	}
	if err := Encrypt(key, iv, reader, writer); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		zap.L().Error("failed flushing writer", zap.Error(err))
		return nil, err
	}
	return buf.Bytes(), nil
}
