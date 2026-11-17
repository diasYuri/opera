package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestQueueSerializesAndCancelsWaiter(t *testing.T) {
	queue := NewQueue()
	release, err := queue.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan error, 1)
	go func() { _, err := queue.Acquire(ctx); waiting <- err }()
	cancel()
	if err := <-waiting; !errors.Is(err, context.Canceled) {
		t.Fatal("waiter did not cancel", err)
	}
	release()
	release()
	next, err := queue.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next()
}

func TestQueueFIFOWithCanceledMiddleWaiter(t *testing.T) {
	q := NewQueue()
	release, err := q.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	order := make(chan int, 3)
	cancelled := make(chan error, 1)
	permits := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	contexts := []context.Context{context.Background(), nil, context.Background()}
	middle, cancel := context.WithCancel(context.Background())
	contexts[1] = middle
	for i := 0; i < 3; i++ {
		i := i
		go func() {
			unlock, err := q.Acquire(contexts[i])
			if err != nil {
				cancelled <- err
				return
			}
			order <- i
			<-permits[i]
			unlock()
		}()
		deadline := time.After(time.Second)
		waiting := false
		for !waiting {
			q.mu.Lock()
			waiting = len(q.waiting) == i+1
			q.mu.Unlock()
			select {
			case <-deadline:
				t.Fatal("waiter did not queue")
			default:
				runtime.Gosched()
			}
		}
	}
	cancel()
	if err := <-cancelled; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	release()
	if first := <-order; first != 0 {
		t.Fatal("queue was not FIFO", first)
	}
	close(permits[0])
	if next := <-order; next != 2 {
		t.Fatal("cancelled waiter blocked queue", next)
	}
	close(permits[2])
}
func TestOwnershipLockRejectsSymlinkAlias(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(workspace, alias); err != nil {
		t.Skip(err)
	}
	release, err := Lock(filepath.Join(root, "locks"), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	if second, err := Lock(filepath.Join(root, "locks"), alias); err == nil {
		_ = second()
		t.Fatal("second process could lock workspace through an alias")
	}
}

func TestOwnershipCannotBeBypassedWithDifferentDataDirectory(t *testing.T) {
	workspace := t.TempDir()
	one, err := Lock(t.TempDir(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = one() }()
	if two, err := Lock(t.TempDir(), workspace); err == nil {
		_ = two()
		t.Fatal("data directory override bypassed ownership")
	}
}
