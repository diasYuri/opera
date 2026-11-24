package updates

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ReplaceCurrent atomically replaces the running executable on Unix systems.
// The caller must supply bytes obtained from a verified release archive.
func ReplaceCurrent(binary []byte) error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("in-place updates are supported on macOS and Linux, not %s", runtime.GOOS)
	}
	if len(binary) == 0 {
		return errors.New("refusing to install an empty executable")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the current Opera executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve the current Opera executable: %w", err)
	}
	info, err := os.Stat(executable)
	if err != nil {
		return fmt.Errorf("inspect the current Opera executable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return errors.New("current Opera path is not an executable regular file")
	}
	temporary, err := os.CreateTemp(filepath.Dir(executable), ".opera-update-*")
	if err != nil {
		return fmt.Errorf("stage the Opera update beside the current executable: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if _, err := temporary.Write(binary); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write the staged Opera executable: %w", err)
	}
	if err := temporary.Chmod(info.Mode().Perm() & 0755); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set staged Opera executable permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("flush the staged Opera executable: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close the staged Opera executable: %w", err)
	}
	if err := os.Rename(temporaryName, executable); err != nil {
		return fmt.Errorf("replace the Opera executable (check write permissions for %s): %w", filepath.Dir(executable), err)
	}
	return nil
}
