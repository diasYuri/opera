package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func testStore(t *testing.T) FileStore {
	t.Helper()
	return FileStore{Directory: filepath.Join(t.TempDir(), "data", "credentials")}
}

func TestFileStoreLifecycle(t *testing.T) {
	store := testStore(t)
	if _, err := store.Get("default"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(store.Directory)); !os.IsNotExist(err) {
		t.Fatal("reading a missing secret created directories")
	}
	if err := store.Set("default", "first secret"); err != nil {
		t.Fatal(err)
	}
	// Reopen a new adapter to verify persistence, not an in-memory cache.
	reopened := FileStore{Directory: store.Directory}
	value, err := reopened.Get("default")
	if err != nil || value != "first secret" {
		t.Fatalf("persisted value: %v", err)
	}
	if err := reopened.Set("default", "updated secret"); err != nil {
		t.Fatal(err)
	}
	value, err = store.Get("default")
	if err != nil || value != "updated secret" {
		t.Fatalf("updated value: %v", err)
	}
	if err := store.Delete("default"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("default"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted credential still exists: %v", err)
	}
}

func TestInvalidReferencesAndSecrets(t *testing.T) {
	store := testStore(t)
	for _, ref := range []string{"", "../secret", "/absolute", "name\n", "-dash"} {
		if err := store.Set(ref, "secret"); err == nil {
			t.Errorf("accepted invalid reference")
		}
	}
	if err := store.Set("valid", ""); err == nil {
		t.Fatal("accepted empty secret")
	}
	if _, err := os.Stat(store.Directory); !os.IsNotExist(err) {
		t.Fatal("invalid input created storage")
	}
}

func TestConcurrentDistinctReferences(t *testing.T) {
	store := testStore(t)
	// Prepare once so this test focuses on independent atomic files.
	if err := store.Set("initial", "secret"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, ref := range []string{"a", "b", "c", "d"} {
		wg.Go(func() {
			if err := store.Set(ref, ref+"-secret"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for _, ref := range []string{"a", "b", "c", "d"} {
		if got, err := store.Get(ref); err != nil || got != ref+"-secret" {
			t.Fatalf("lost independent update: %v", err)
		}
	}
}
