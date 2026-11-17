//go:build windows

package ui

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

	"github.com/diasYuri/opera/internal/app"
	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

func TestConPTYChild(t *testing.T) {
	if os.Getenv("OPERA_CONPTY_CHILD") != "1" {
		return
	}
	before, err := term.GetState(int(os.Stdin.Fd()))
	if err != nil {
		os.Exit(21)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = Chat(ctx, terminalExecutor{}, os.Stdin, os.Stdout, app.Request{Prompt: "fixture"}, true)
	if err != nil {
		os.Exit(22)
	}
	after, err := term.GetState(int(os.Stdin.Fd()))
	if err != nil || !reflect.DeepEqual(before, after) {
		os.Exit(23)
	}
	_, _ = fmt.Fprintln(os.Stdout, "CONPTY_RESTORED")
	os.Exit(0)
}
func TestWindowsConPTYRestoresConsoleAndAlternateScreen(t *testing.T) {
	t.Setenv("OPERA_CONPTY_CHILD", "1")
	t.Setenv("NO_COLOR", "")
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = inputRead.Close(); _ = inputWrite.Close() }()
	outputRead, outputWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
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
	var once sync.Once
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := outputRead.Read(buffer)
			if n > 0 {
				mu.Lock()
				captured.Write(buffer[:n])
				if strings.Contains(captured.String(), "PTY_RESPONSE_MARKER") {
					once.Do(func() { close(visible) })
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
		t.Fatal("ConPTY never displayed the response")
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
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(captured.String(), "\x1b[?1049h") || !strings.Contains(captured.String(), "\x1b[?1049l") {
		t.Fatal("alternate screen was not entered and restored")
	}
}
