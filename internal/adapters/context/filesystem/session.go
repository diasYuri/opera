package contextfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type SessionContext struct {
	Data, Workspace string
}

func (s SessionContext) WriteContext(ctx context.Context, session, markdown string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Join(s.Data, "sessions", fmt.Sprintf("%x", sha256.Sum256([]byte(s.Workspace+"\x00"+session))))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "context.md")
	if markdown == "" {
		markdown = "# Session context\n"
	}
	raw := []byte(markdown)
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Size() == int64(len(raw)) {
		if current, err := os.ReadFile(path); err == nil && bytes.Equal(current, raw) {
			return nil
		}
	}
	file, err := os.CreateTemp(dir, ".context-*")
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			return
		}
		if cleanup := os.Remove(file.Name()); cleanup != nil && !errors.Is(cleanup, os.ErrNotExist) {
			err = errors.Join(err, cleanup)
		}
	}()
	if _, err := file.Write(raw); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
