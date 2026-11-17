package cli_test

import (
	"golang.org/x/sys/windows"
	"os"
	"strconv"
	"strings"
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
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			continue
		}
		var code uint32
		err = windows.GetExitCodeProcess(handle, &code)
		_ = windows.CloseHandle(handle)
		if err != nil || code == 259 {
			t.Fatal("MCP process survived scope close", pid, err)
		}
	}
}
