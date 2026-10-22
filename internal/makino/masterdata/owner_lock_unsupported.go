//go:build !(linux || darwin || dragonfly || freebsd || netbsd || openbsd || solaris || windows)

package masterdata

import (
	"errors"
	"os"
)

func lockOwnerFile(*os.File) error {
	return errors.New("master resource ownership locking is unsupported on this operating system")
}
