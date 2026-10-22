package masterdata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	mastercrypto "github.com/vertesan/solis-server/internal/utils/crypto"
)

// ensureOriginalFile recovers packs deleted by older customization imports.
// Only the original manifest supplies download metadata; official filenames are
// opaque and must not be compared with the SHA-256 convention used by imports.
func ensureOriginalFile(ctx context.Context, pack *ptransaction.MasterTagPack) (retErr error) {
	if pack == nil || !encryptedFileName.MatchString(pack.FileName) || pack.FileSize <= 0 || pack.Type == "" {
		return errors.New("invalid original master pack metadata")
	}
	directory := config.GetDBDir()
	if err := validateOriginalFile(ctx, directory, pack.FileName, pack); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("validate existing original master %q: %w", pack.Type, err)
	}

	downloadURL, err := url.Parse(pack.DownloadUrl)
	if err != nil || downloadURL.Host == "" || downloadURL.User != nil ||
		(downloadURL.Scheme != "http" && downloadURL.Scheme != "https") {
		return fmt.Errorf("original master %q is missing and has an invalid download URL", pack.Type)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL.String(), nil)
	if err != nil {
		return fmt.Errorf("prepare original master %q download: %w", pack.Type, err)
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download missing original master %q: %w", pack.Type, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download missing original master %q: HTTP %d", pack.Type, response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != int64(pack.FileSize) {
		return fmt.Errorf("download original master %q: content length %d, want %d", pack.Type, response.ContentLength, pack.FileSize)
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	temporaryName := ".master-original-" + uuid.NewString() + ".tmp"
	temporary, err := root.OpenFile(temporaryName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create original master download: %w", err)
	}
	defer func() {
		if temporary != nil {
			retErr = errors.Join(retErr, temporary.Close())
		}
		if err := root.Remove(temporaryName); err != nil && !errors.Is(err, os.ErrNotExist) {
			retErr = errors.Join(retErr, fmt.Errorf("remove original master temporary file: %w", err))
		}
	}()

	written, err := io.Copy(temporary, io.LimitReader(response.Body, int64(pack.FileSize)+1))
	if err != nil {
		return fmt.Errorf("read original master %q download: %w", pack.Type, err)
	}
	if written != int64(pack.FileSize) {
		return fmt.Errorf("download original master %q: size %d, want %d", pack.Type, written, pack.FileSize)
	}
	if err := temporary.Chmod(0644); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync original master download: %w", err)
	}
	err = temporary.Close()
	temporary = nil
	if err != nil {
		return err
	}
	if err := validateOriginalFile(ctx, directory, temporaryName, pack); err != nil {
		return fmt.Errorf("validate downloaded original master %q: %w", pack.Type, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Link publishes complete bytes without replacing a file created concurrently.
	if err := root.Link(temporaryName, pack.FileName); err != nil {
		if errors.Is(err, os.ErrExist) {
			return validateOriginalFile(ctx, directory, pack.FileName, pack)
		}
		return fmt.Errorf("publish original master %q: %w", pack.Type, err)
	}
	return syncDirectory(directory)
}

func validateOriginalFile(ctx context.Context, directory, name string, pack *ptransaction.MasterTagPack) error {
	file, err := safeFile(directory, name)
	if err != nil {
		return err
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if err := errors.Join(statErr, closeErr); err != nil {
		return err
	}
	if info.Size() != int64(pack.FileSize) {
		return fmt.Errorf("original master size %d, want %d; refusing to overwrite existing file", info.Size(), pack.FileSize)
	}
	path := filepath.Join(directory, name)
	// Schema inspection and row reads alone do not detect damaged indexes or
	// auxiliary pages. Self-comparison also runs SQLite/cipher integrity and
	// foreign-key checks without comparing official rows to customized data.
	if err := mastercrypto.ValidateMasterDb(ctx, path, path, pack.CryptoKey, pack.Type); err != nil {
		return err
	}
	schema, err := mastercrypto.InspectMasterDb(ctx, path, pack.CryptoKey, pack.Type)
	if err != nil {
		return err
	}
	var count int64
	if err := mastercrypto.IterateMasterDb(ctx, path, pack.CryptoKey, pack.Type, func(map[string]any) error {
		count++
		return nil
	}); err != nil {
		return err
	}
	if count != schema.RowCount {
		return fmt.Errorf("original master row count %d, want %d", count, schema.RowCount)
	}
	return nil
}
