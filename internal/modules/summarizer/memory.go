package summarizer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	contextdata "github.com/diasYuri/opera/internal/modules/context"
	"github.com/diasYuri/opera/internal/modules/records"
)

const bucket = "session-context"
const privateBucket = "run-context-projections"

type Store interface {
	Get(context.Context, string, string) (records.Document, error)
	CAS(context.Context, string, string, uint64, any) error
}

type File interface {
	WriteContext(context.Context, string, string) error
}

type Memory struct {
	Store Store
	File  File
}

type Context struct {
	Markdown   string
	Projection Projection
	Rewritten  Projection `json:",omitzero"`
	HeadRun    string     `json:",omitempty"`
	CommitID   string     `json:",omitempty"`
	Base       string     `json:",omitempty"`
	Version    uint64     `json:"-"`
}

// ContentID binds a private projection to the shared context it summarized.
func (c Context) ContentID() string {
	raw, _ := json.Marshal(struct {
		Markdown   string
		Projection Projection
		Rewritten  Projection
	}{c.Markdown, c.Projection, c.Rewritten})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func (m Memory) Read(ctx context.Context, session string) (Context, error) {
	return m.read(ctx, bucket, session)
}

func (m Memory) ReadPrivate(ctx context.Context, run string) (Context, error) {
	return m.read(ctx, privateBucket, run)
}

func (m Memory) read(ctx context.Context, namespace, key string) (Context, error) {
	doc, err := m.Store.Get(ctx, namespace, key)
	if errors.Is(err, sql.ErrNoRows) {
		return Context{}, nil
	}
	if err != nil {
		return Context{}, err
	}
	var value Context
	err = json.Unmarshal(doc.Data, &value)
	value.Version = doc.Version
	return value, err
}

func (m Memory) Commit(ctx context.Context, session string, value Context) error {
	return m.commit(ctx, bucket, session, value)
}

func (m Memory) CommitPrivate(ctx context.Context, run string, value Context) error {
	return m.commit(ctx, privateBucket, run, value)
}

func (m Memory) commit(ctx context.Context, namespace, key string, value Context) error {
	if len(value.Markdown) > MemoryTokens {
		return errors.New("session context exceeds its 5000-byte ceiling")
	}
	return m.Store.CAS(ctx, namespace, key, value.Version, value)
}

// Materialize repairs a missing or out-of-date file from the authoritative
// record. Checking the version after the rename also handles concurrent writers.
func (m Memory) Materialize(ctx context.Context, session string) error {
	for range 4 {
		value, err := m.Read(ctx, session)
		if err != nil {
			return err
		}
		if err := m.File.WriteContext(ctx, session, value.Markdown); err != nil {
			return fmt.Errorf("publish context.md: %w", err)
		}
		latest, err := m.Read(ctx, session)
		if err != nil {
			return err
		}
		if latest.Version == value.Version {
			return nil
		}
	}
	return errors.New("session context is being updated concurrently")
}

func (m Memory) Note(ctx context.Context, session, note string) error {
	note = strings.TrimSpace(contextdata.RedactText(note))
	if note == "" || len(note) > 1000 {
		return errors.New("session note must have between 1 and 1000 UTF-8 bytes")
	}
	for range 4 {
		value, err := m.Read(ctx, session)
		if err != nil {
			return err
		}
		entry := "\n### Agent note\n\n" + note + "\n"
		if strings.Contains(value.Markdown, entry) {
			return nil
		}
		value.Markdown += entry
		err = m.Commit(ctx, session, value)
		if !errors.Is(err, records.ErrConflict) {
			return err
		}
	}
	return records.ErrConflict
}
