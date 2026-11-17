//go:build !windows

package tui

import (
	"context"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/diasYuri/opera/internal/modules/execution"
	"golang.org/x/term"
)

func TestTerminalRestoredWithAlternateScreen(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = master.Close(); _ = slave.Close() }()
	if err := pty.Setsize(slave, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	before, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var mu sync.Mutex
	var captured strings.Builder
	visible := make(chan struct{})
	var once sync.Once
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := master.Read(buffer)
			if n > 0 {
				mu.Lock()
				_, _ = captured.Write(buffer[:n])
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
	done := make(chan error, 1)
	go func() {
		_, err := Chat(ctx, terminalExecutor{}, slave, slave, execution.Request{Prompt: "fixture"})
		done <- err
	}()
	select {
	case <-visible:
	case <-ctx.Done():
		t.Fatal("terminal never displayed response")
	}
	// Wait for the final result to settle in the UI event loop before exit.
	if _, err := io.WriteString(master, "\x04"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("terminal did not exit")
	}
	after, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("terminal mode was not restored")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(captured.String(), "\x1b[?1049h") || !strings.Contains(captured.String(), "\x1b[?1049l") {
		t.Fatal("alternate screen was not entered and restored")
	}
}
