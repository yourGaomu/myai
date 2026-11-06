//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package trust

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openStoreLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		_ = file.Close()
		return nil, errors.New("trust lock must be a private regular file")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		_ = file.Close()
		return nil, errors.New("trust lock must be owned by the current user without hard links")
	}
	return file, nil
}

func lockStoreFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX)
		if err != unix.EINTR {
			return err
		}
	}
}

func unlockStoreFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
