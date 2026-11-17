package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceCompletionsConfineDiscovery(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "guide.txt"), []byte("guide"), 0600); err != nil {
		t.Fatal(err)
	}
	items := completeWorkspaceFiles(context.Background(), root, "@gui")
	if len(items) != 1 || items[0].Value != "@guide.txt " {
		t.Fatalf("file suggestions = %#v", items)
	}
	if items := completeWorkspaceFiles(context.Background(), root, "@../"); len(items) != 0 {
		t.Fatal("escaped workspace")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if items := completeWorkspaceFiles(canceled, root, "@"); len(items) != 0 {
		t.Fatal("discovery ignored cancellation")
	}
}
