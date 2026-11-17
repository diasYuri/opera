package contextfiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedContextBudgetIgnoreAndRedaction(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"AGENTS.md": "Root instructions", "src/AGENTS.md": "Scoped instructions", "src/main.go": "package main\nAPI_KEY=private-value", ".gitignore": "ignored.txt\n", "ignored.txt": "hidden"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	out, err := Load(context.Background(), root, []string{"src/main.go"}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sources) != 3 || !strings.Contains(out.Text, "[REDACTED]") || strings.Contains(out.Text, "private-value") || strings.Index(out.Text, "Root instructions") > strings.Index(out.Text, "Scoped instructions") {
		t.Fatal(out)
	}
	for _, paths := range [][]string{{"ignored.txt"}, {"../outside"}} {
		if _, err := Load(context.Background(), root, paths, 4096); err == nil {
			t.Fatal("unsafe context accepted", paths)
		}
	}
	if _, err := Load(context.Background(), root, []string{"src/main.go"}, 20); err == nil {
		t.Fatal("budget ignored")
	}
}
func TestImageIsTransientAndRequiresVision(t *testing.T) {
	root := t.TempDir()
	data := append([]byte{137, 80, 78, 71, 13, 10, 26, 10}, make([]byte, 20)...)
	if err := os.WriteFile(filepath.Join(root, "a.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := Load(context.Background(), root, []string{"a.png"}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "sha256=") || out.Sources[0].Kind != "image" {
		t.Fatal(out)
	}
	if _, err := Images(context.Background(), root, []string{"a.png"}, false, 4096); err == nil {
		t.Fatal("unsupported vision allowed")
	}
	blocks, err := Images(context.Background(), root, []string{"a.png"}, true, 4096)
	if err != nil || len(blocks) != 1 {
		t.Fatal(err)
	}
}
