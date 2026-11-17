// Package sqlite owns Opera records alongside the public SDK checkpoint store.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/opera/internal/modules/accounting"
	"github.com/diasYuri/opera/internal/modules/records"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var _ accounting.UsageStore = (*Store)(nil)

type Store struct {
	db        *sql.DB
	Workspace string
}
type Document = records.Document

func Open(ctx context.Context, path, workspace string) (*Store, error) {
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("opera database must be a regular file")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, Workspace: workspace}
	if err := s.migrate(ctx, path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) migrate(ctx context.Context, path string) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='opera_schema'").Scan(&exists); err != nil {
		return err
	}
	version := 0
	if exists > 0 {
		if err := s.db.QueryRowContext(ctx, "SELECT version FROM opera_schema").Scan(&version); err != nil {
			return err
		}
	}
	if version > 1 {
		return fmt.Errorf("opera schema %d is newer than supported version 1", version)
	}
	if version == 1 {
		var check string
		if err := s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil {
			return err
		}
		if check != "ok" {
			return fmt.Errorf("opera database integrity check failed")
		}
		rows, err := s.db.QueryContext(ctx, "SELECT workspace,bucket,key,data,version FROM opera_documents LIMIT 0")
		if err != nil {
			return err
		}
		return rows.Close()
	}
	backup := path + ".before-opera-v1.bak"
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", backup); err != nil {
			return fmt.Errorf("backup before migration: %w", err)
		}
		if err := os.Chmod(backup, 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, statement := range []string{"CREATE TABLE IF NOT EXISTS opera_schema (version INTEGER NOT NULL)", "INSERT INTO opera_schema(version) SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM opera_schema)", "UPDATE opera_schema SET version=1", "CREATE TABLE IF NOT EXISTS opera_documents (workspace TEXT NOT NULL,bucket TEXT NOT NULL,key TEXT NOT NULL,data BLOB NOT NULL,version INTEGER NOT NULL,PRIMARY KEY(workspace,bucket,key))"} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Get(ctx context.Context, bucket, key string) (Document, error) {
	d := Document{Key: key}
	err := s.db.QueryRowContext(ctx, "SELECT data,version FROM opera_documents WHERE workspace=? AND bucket=? AND key=?", s.Workspace, bucket, key).Scan(&d.Data, &d.Version)
	return d, err
}
func (s *Store) List(ctx context.Context, bucket string) ([]Document, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key,data,version FROM opera_documents WHERE workspace=? AND bucket=? ORDER BY key", s.Workspace, bucket)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.Key, &d.Data, &d.Version); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Store) Put(ctx context.Context, bucket, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO opera_documents(workspace,bucket,key,data,version) VALUES(?,?,?,?,1) ON CONFLICT(workspace,bucket,key) DO UPDATE SET data=excluded.data,version=opera_documents.version+1", s.Workspace, bucket, key, raw)
	return err
}
func (s *Store) CAS(ctx context.Context, bucket, key string, version uint64, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if version == 0 {
		_, err = s.db.ExecContext(ctx, "INSERT INTO opera_documents(workspace,bucket,key,data,version) VALUES(?,?,?,?,1)", s.Workspace, bucket, key, raw)
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE opera_documents SET data=?,version=version+1 WHERE workspace=? AND bucket=? AND key=? AND version=?", raw, s.Workspace, bucket, key, version)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return fmt.Errorf("opera record version conflict")
	}
	return err
}
