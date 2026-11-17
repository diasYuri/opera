//go:build !windows

package workspace

import (
	"errors"
	"os"
	"syscall"
)

func lockConflict(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

func lockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// Lock the directory inode as well as the data-directory lock, so configuring a
// different OPERA_DATA_DIR cannot bypass workspace ownership.
func lockWorkspace(path string) (func() error, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file.Close, nil
}
