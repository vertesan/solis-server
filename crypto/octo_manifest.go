package crypto

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"io"
	"vertesan/solis-server/utils/rich"
)

func EncryptOctoManifest(reader io.Reader, key []byte) ([]byte, error) {
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		rich.Error("failed generating 16 random bytes iv: %v", err)
		return nil, err
	}
	buf := &bytes.Buffer{}
	writer := bufio.NewWriter(buf)
	writer.Write(iv)
	if err := writer.Flush(); err != nil {
		rich.Error("failed flushing writer: %v", err)
		return nil, err
	}
	Encrypt(key, iv, reader, writer)
	if err := writer.Flush(); err != nil {
		rich.Error("failed flushing writer: %v", err)
		return nil, err
	}
	return buf.Bytes(), nil
}
