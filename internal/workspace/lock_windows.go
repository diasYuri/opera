package workspace

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func lockConflict(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }

func lockFile(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}

func lockWorkspace(path string) (func() error, error) {
	dir := filepath.Join(path, ".opera")
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("workspace state directory must not be a symlink")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock := filepath.Join(dir, "ownership.lock")
	if info, err := os.Lstat(lock); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, fmt.Errorf("workspace lock must be a regular file")
	}
	file, err := os.OpenFile(lock, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file.Close, nil
}
