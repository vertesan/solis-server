package masterdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/vertesan/solis-server/internal/utils/config"
)

var ErrOwnershipBusy = errors.New("master resources are owned by a running server or another offline import")

// Ownership fences server startup and offline imports sharing the resource
// directory. The operating system releases ownership if the process exits.
type Ownership struct {
	mu   sync.Mutex
	file *os.File
}

// AcquireOwnership fails immediately if another participating process owns the
// resources. Every server must hold ownership throughout its database lifetime.
func AcquireOwnership() (*Ownership, error) {
	directory := config.GetDBDir()
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, fmt.Errorf("create master resource directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("master resource directory must be a regular directory")
	}
	name := filepath.Join(directory, ".master-owner.lock")
	if info, err := os.Lstat(name); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("master ownership lock must be a regular file")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open master ownership lock: %w", err)
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = file.Close()
		}
	}()
	if err := lockOwnerFile(file); err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	current, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return nil, errors.New("master ownership lock changed or is not a regular file")
	}
	accepted = true
	return &Ownership{file: file}, nil
}

// Close does not remove the lock file: unlinking it would let another process
// lock a new inode while a previous owner still holds the original one.
func (o *Ownership) Close() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.file == nil {
		return nil
	}
	err := o.file.Close()
	o.file = nil
	return err
}

func (o *Ownership) withLock(fn func() error) error {
	if o == nil {
		return errors.New("master resource ownership is required")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.file == nil {
		return errors.New("master resource ownership has been released")
	}
	return fn()
}
