//go:build !windows

package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreRejectsUnsafePermissionsAndSymlinks(t *testing.T) {
	store := testStore(t)
	if err := store.Set("default", "secret"); err != nil {
		t.Fatal(err)
	}
	path, err := store.path("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("default"); err == nil {
		t.Fatal("accepted readable-by-others credential")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "replacement"); err == nil {
		t.Fatal("accepted credential symlink")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "original" {
		t.Fatal("modified symlink target")
	}
	if err := os.Chmod(store.Directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("other", "secret"); err == nil {
		t.Fatal("accepted insecure credential directory")
	}
}
