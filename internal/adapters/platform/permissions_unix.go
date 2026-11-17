//go:build !windows

package platform

import (
	"fmt"
	"os"
	"syscall"
)

func Protect(path string, directory bool) error {
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	return os.Chmod(path, mode)
}

func Check(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("credential path must be a regular file or directory, never a symlink")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("credential path must be owned by the current user with mode 0700 (directory) or 0600 (file)")
	}
	return nil
}
