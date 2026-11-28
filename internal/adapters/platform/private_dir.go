package platform

import (
	"os"
	"path/filepath"
)

// EnsurePrivateDir protects a newly created directory and validates existing
// directories without changing their ownership or permissions.
func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0700); err == nil {
		if err := Protect(path, true); err != nil {
			return err
		}
	} else if !os.IsExist(err) {
		return err
	}
	return Check(path, true)
}
