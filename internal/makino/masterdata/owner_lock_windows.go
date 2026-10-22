package masterdata

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockOwnerFile(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrOwnershipBusy
	}
	if err != nil {
		return fmt.Errorf("acquire master resource ownership: %w", err)
	}
	return nil
}
