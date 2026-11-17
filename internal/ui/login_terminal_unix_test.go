//go:build !windows

package ui

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/diasYuri/opera/internal/app"
)

func TestTerminalLoginSubmenuAndCancellation(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = master.Close(); _ = slave.Close() }()
	if err := pty.Setsize(slave, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	markers := []string{"/login connect", "Connect a provider", "Finish sign-in in your browser", "Login cancelled"}
	stages := make([]chan struct{}, len(markers))
	for i := range stages {
		stages[i] = make(chan struct{})
	}
	var mu sync.Mutex
	var captured strings.Builder
	go func() {
		seen := make([]bool, len(markers))
		buffer := make([]byte, 4096)
		for {
			n, err := master.Read(buffer)
			if n > 0 {
				mu.Lock()
				_, _ = captured.Write(buffer[:n])
				text := captured.String()
				for i, marker := range markers {
					if !seen[i] && strings.Contains(text, marker) {
						seen[i] = true
						close(stages[i])
					}
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	e := &loginExecutor{providers: []app.LoginProvider{{ID: "openai", Name: "OpenAI", Description: "ChatGPT subscription"}}, waitForCancel: true}
	done := make(chan error, 1)
	go func() { _, err := Chat(ctx, e, slave, slave, app.Request{}, true); done <- err }()
	wait := func(stage int) {
		t.Helper()
		select {
		case <-stages[stage]:
		case <-ctx.Done():
			mu.Lock()
			defer mu.Unlock()
			t.Fatalf("terminal never displayed %q: %s", markers[stage], captured.String())
		}
	}
	send := func(text string) {
		t.Helper()
		if _, err := io.WriteString(master, text); err != nil {
			t.Fatal(err)
		}
	}
	wait(0)
	send("/login\r")
	wait(1)
	send("\r")
	wait(2)
	if err := pty.Setsize(slave, &pty.Winsize{Rows: 12, Cols: 40}); err != nil {
		t.Fatal(err)
	}
	send("\x1b")
	wait(3)
	send("\x04")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("session did not exit after cancelled login")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(captured.String(), "\x1b[?1049h") || !strings.Contains(captured.String(), "\x1b[?1049l") {
		t.Fatal("alternate screen was not entered and restored")
	}
}
