//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package trust

import (
	"errors"
	"os"
)

func openStoreLock(string) (*os.File, error) {
	return nil, errors.New("cross-process trust locks are unsupported on this platform")
}
func lockStoreFile(*os.File) error {
	return errors.New("cross-process trust locks are unsupported on this platform")
}
func unlockStoreFile(*os.File) error { return nil }
