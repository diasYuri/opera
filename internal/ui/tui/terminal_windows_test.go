//go:build windows

package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/diasYuri/opera/internal/modules/execution"
	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

func TestConPTYChild(t *testing.T) {
	if os.Getenv("OPERA_CONPTY_CHILD") != "1" {
		return
	}
	// The test runner can redirect its standard streams. Open the console
	// attached to this child explicitly so Chat exercises real console modes.
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		os.Exit(24)
	}
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		os.Exit(25)
	}
	before, err := term.GetState(int(input.Fd()))
	if err != nil {
		os.Exit(21)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = Chat(ctx, terminalExecutor{}, input, output, execution.Request{Prompt: "fixture"}, true)
	if err != nil {
		_, _ = fmt.Fprintln(output, err)
		os.Exit(22)
	}
	after, err := term.GetState(int(input.Fd()))
	if err != nil || !reflect.DeepEqual(before, after) {
		os.Exit(23)
	}
	_, _ = fmt.Fprintln(output, "CONPTY_RESTORED")
	_ = output.Close()
	_ = input.Close()
	os.Exit(0)
}
func TestWindowsConPTYRestoresConsoleAndAlternateScreen(t *testing.T) {
	t.Setenv("OPERA_CONPTY_CHILD", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	inputRead, inputWrite := conPTYPipe(t)
	defer func() { _ = inputRead.Close(); _ = inputWrite.Close() }()
	outputRead, outputWrite := conPTYPipe(t)
	defer func() { _ = outputRead.Close(); _ = outputWrite.Close() }()
	var console windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: 80, Y: 24}, windows.Handle(inputRead.Fd()), windows.Handle(outputWrite.Fd()), 0, &console); err != nil {
		t.Fatal("ConPTY is required on supported Windows runners", err)
	}
	defer windows.ClosePseudoConsole(console)
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatal(err)
	}
	defer attributes.Delete()
	// HPCON is passed by value, as specified in Microsoft's ConPTY startup sample:
	// https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session
	update := windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")
	result, _, callErr := update.Call(uintptr(unsafe.Pointer(attributes.List())), 0, windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(console), unsafe.Sizeof(console), 0, 0)
	if result == 0 {
		t.Fatal(callErr)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command, err := windows.UTF16PtrFromString(windows.EscapeArg(binary) + " -test.run=^TestConPTYChild$")
	if err != nil {
		t.Fatal(err)
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	var process windows.ProcessInformation
	if err := windows.CreateProcess(nil, command, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &process); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = windows.TerminateProcess(process.Process, 1)
		_ = windows.CloseHandle(process.Thread)
		_ = windows.CloseHandle(process.Process)
	}()
	_ = inputRead.Close()
	_ = outputWrite.Close()
	var mu sync.Mutex
	var captured strings.Builder
	visible := make(chan struct{})
	restored := make(chan struct{})
	var responseOnce, restoreOnce sync.Once
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := outputRead.Read(buffer)
			if n > 0 {
				mu.Lock()
				captured.Write(buffer[:n])
				if strings.Contains(captured.String(), "PTY_RESPONSE_MARKER") {
					responseOnce.Do(func() { close(visible) })
				}
				if strings.Contains(captured.String(), "CONPTY_RESTORED") {
					restoreOnce.Do(func() { close(restored) })
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-visible:
	case <-time.After(15 * time.Second):
		var code uint32
		exitErr := windows.GetExitCodeProcess(process.Process, &code)
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("ConPTY never displayed the response (child exit code %d, status error %v): %q", code, exitErr, captured.String())
	}
	if err := windows.ResizePseudoConsole(console, windows.Coord{X: 100, Y: 28}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(inputWrite, "\x04"); err != nil {
		t.Fatal(err)
	}
	status, err := windows.WaitForSingleObject(process.Process, 15000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("ConPTY child failed to exit", status, err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(process.Process, &code); err != nil || code != 0 {
		t.Fatal("console not restored", code, err)
	}
	select {
	case <-restored:
	case <-time.After(5 * time.Second):
		t.Fatal("ConPTY did not capture the restored-console marker")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(captured.String(), "\x1b[?1049h") || !strings.Contains(captured.String(), "\x1b[?1049l") {
		t.Fatal("alternate screen was not entered and restored")
	}
}

func conPTYPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	// Use synchronous, non-inheritable handles for ConPTY's transport.
	var read, write windows.Handle
	if err := windows.CreatePipe(&read, &write, nil, 0); err != nil {
		t.Fatal(err)
	}
	return os.NewFile(uintptr(read), "conpty-read"), os.NewFile(uintptr(write), "conpty-write")
}
