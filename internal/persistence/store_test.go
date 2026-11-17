package persistence

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationBackupVersionAndCAS(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "opera.db")
	s, err := Open(ctx, path, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".before-opera-v1.bak"); err != nil {
		t.Fatal("migration backup absent", err)
	}
	if err := s.CAS(ctx, "children", "id", 0, map[string]string{"status": "queued"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CAS(ctx, "children", "id", 0, "duplicate"); err == nil {
		t.Fatal("duplicate create succeeded")
	}
	if err := s.CAS(ctx, "children", "id", 2, "stale"); err == nil {
		t.Fatal("stale version succeeded")
	}
	if err := s.CAS(ctx, "children", "id", 1, "updated"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE opera_schema SET version=99"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(ctx, path, "workspace"); err == nil {
		_ = store.Close()
		t.Fatal("future schema accepted")
	}
}
func TestCorruptDatabaseFailsBeforeUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(context.Background(), path, "workspace"); err == nil {
		_ = s.Close()
		t.Fatal("corrupt database accepted")
	}
}
