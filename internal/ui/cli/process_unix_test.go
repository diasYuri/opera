//go:build !windows

package cli_test

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func assertMCPProcessesExited(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range strings.Fields(string(raw)) {
		pid, err := strconv.Atoi(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(pid, 0); err == nil {
			t.Fatal("MCP process survived scope close", pid)
		}
	}
}
