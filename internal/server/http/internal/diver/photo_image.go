package diver

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Metadata and the on-disk image must be read/published together. Bounded locks
// avoid retaining a lock for every image ever uploaded to this server.
var photoObjectLocks [64]sync.Mutex

func LockPhotoObject(name string) func() {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(name))
	lock := &photoObjectLocks[hash.Sum32()%uint32(len(photoObjectLocks))]
	lock.Lock()
	return lock.Unlock
}

// PhotoImagePublication keeps a hard link to the previous image until its new
// metadata has committed, without copying the image into another memory buffer.
type PhotoImagePublication struct {
	destination string
	previous    string
	published   bool
}

func (p *PhotoImagePublication) Commit() error {
	if p.previous == "" {
		return nil
	}
	return os.Remove(p.previous)
}

func (p *PhotoImagePublication) Rollback() error {
	if !p.published {
		return p.Commit()
	}
	if p.previous != "" {
		return os.Rename(p.previous, p.destination)
	}
	err := os.Remove(p.destination)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// PublishPhotoImage atomically replaces a validated image path. The caller must
// hold LockPhotoObject until it commits or rolls back the associated metadata.
func PublishPhotoImage(destination string, body []byte, modifiedAt time.Time) (publication *PhotoImagePublication, retErr error) {
	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create photo directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".photo-upload-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary photo: %w", err)
	}
	temporaryPath := temporary.Name()
	pending := &PhotoImagePublication{destination: destination}
	defer func() {
		if temporary != nil {
			retErr = errors.Join(retErr, temporary.Close())
		}
		if !pending.published {
			if err := os.Remove(temporaryPath); !os.IsNotExist(err) {
				retErr = errors.Join(retErr, err)
			}
		}
		if retErr != nil {
			retErr = errors.Join(retErr, pending.Rollback())
		}
	}()
	if _, err := temporary.Write(body); err != nil {
		return nil, fmt.Errorf("failed to write temporary photo: %w", err)
	}
	if err := temporary.Chmod(0o640); err != nil {
		return nil, fmt.Errorf("failed to set photo permissions: %w", err)
	}
	if err := os.Chtimes(temporaryPath, modifiedAt, modifiedAt); err != nil {
		return nil, fmt.Errorf("failed to set photo timestamps: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return nil, fmt.Errorf("failed to sync temporary photo: %w", err)
	}
	if err := temporary.Close(); err != nil {
		temporary = nil
		return nil, fmt.Errorf("failed to close temporary photo: %w", err)
	}
	temporary = nil
	// The temporary filename was exclusively created, so its backup suffix is
	// also collision-safe. Link keeps the old published path readable until rename.
	if err := os.Link(destination, temporaryPath+".previous"); err == nil {
		pending.previous = temporaryPath + ".previous"
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to preserve previous photo: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return nil, fmt.Errorf("failed to publish photo: %w", err)
	}
	pending.published = true
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return nil, fmt.Errorf("failed to open photo directory: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, directoryHandle.Close()) }()
	if err := directoryHandle.Sync(); err != nil {
		return nil, fmt.Errorf("failed to sync photo directory: %w", err)
	}
	return pending, nil
}
