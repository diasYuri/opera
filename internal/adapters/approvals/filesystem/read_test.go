package approvalfiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadConfinesApprovalObservation(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(name, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	path, data, err := Read(context.Background(), root, "existing.txt")
	if err != nil || path != name || string(data) != "old" {
		t.Fatalf("read = %q %q %v", path, data, err)
	}
	_, data, err = Read(context.Background(), root, "missing.txt")
	if err != nil || !strings.Contains(string(data), "does not exist") {
		t.Fatalf("missing file = %q %v", data, err)
	}
	if _, _, err := Read(context.Background(), root, "../outside.txt"); err == nil {
		t.Fatal("escaped workspace")
	}
	if _, _, err := Read(context.Background(), root, "."); err == nil {
		t.Fatal("read directory")
	}
	if err := os.WriteFile(name, make([]byte, (1<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(context.Background(), root, name); err == nil {
		t.Fatal("read oversized file")
	}
}
