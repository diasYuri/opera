//go:build !windows

package workspace

import (
	"fmt"
	"os"
	"syscall"
)

func workspaceIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("workspace identity unavailable")
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
