// Package workspace owns process-scoped workspace locks and cancellable writes.
package workspace

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type waiter struct {
	ready   chan struct{}
	granted bool
}
type Queue struct {
	mu      sync.Mutex
	busy    bool
	waiting []*waiter
}

func NewQueue() *Queue { return &Queue{} }
func (q *Queue) advance() {
	if len(q.waiting) == 0 {
		q.busy = false
		return
	}
	w := q.waiting[0]
	q.waiting = q.waiting[1:]
	w.granted = true
	close(w.ready)
}
func (q *Queue) release() { q.mu.Lock(); defer q.mu.Unlock(); q.advance() }
func (q *Queue) Acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w := &waiter{ready: make(chan struct{})}
	q.mu.Lock()
	if !q.busy {
		q.busy = true
		w.granted = true
		close(w.ready)
	} else {
		q.waiting = append(q.waiting, w)
	}
	q.mu.Unlock()
	select {
	case <-ctx.Done():
		q.mu.Lock()
		if w.granted {
			q.advance()
		} else {
			for i, item := range q.waiting {
				if item == w {
					q.waiting = append(q.waiting[:i], q.waiting[i+1:]...)
					break
				}
			}
		}
		q.mu.Unlock()
		return nil, ctx.Err()
	case <-w.ready:
		if err := ctx.Err(); err != nil {
			q.release()
			return nil, err
		}
		var once sync.Once
		return func() { once.Do(q.release) }, nil
	}
}

func Lock(directory, canonicalWorkspace string) (func() error, error) {
	ownership, err := lockWorkspace(canonicalWorkspace)
	if err != nil {
		if lockConflict(err) {
			return nil, fmt.Errorf("resource %q is locked by another Opera process", canonicalWorkspace)
		}
		return nil, fmt.Errorf("workspace is already in use or cannot be locked: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = ownership()
		}
	}()

	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	identity, err := workspaceIdentity(canonicalWorkspace)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(directory, fmt.Sprintf("%x.lock", sha256.Sum256([]byte(identity))))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("workspace is already in use by another Opera process: %w", err)
	}
	success = true
	return func() error { return errors.Join(file.Close(), ownership()) }, nil
}
