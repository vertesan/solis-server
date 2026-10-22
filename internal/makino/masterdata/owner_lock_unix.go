//go:build linux || darwin || dragonfly || freebsd || netbsd || openbsd || solaris

package masterdata

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockOwnerFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return ErrOwnershipBusy
		}
		if err != nil {
			return fmt.Errorf("acquire master resource ownership: %w", err)
		}
		return nil
	}
}
