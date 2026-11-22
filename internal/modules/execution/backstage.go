package execution

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"

	"github.com/diasYuri/opera/internal/modules/records"
)

type backstageKey struct {
	session string
	child   string
}

type backstageTask struct {
	key     backstageKey
	run     string
	ctx     context.Context
	cancel  context.CancelFunc
	runJob  func(context.Context) error
	done    chan struct{}
	err     error
	urgent  bool
	started bool
}

func (t *backstageTask) wait(ctx context.Context) error {
	select {
	case <-t.done:
		return t.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *backstageTask) finish(err error) {
	t.err = err
	t.cancel()
	close(t.done)
}

type backstageLane struct {
	active  *backstageTask
	settled *backstageTask
}

// Backstage owns process-local work. Each lane holds an active or queued
// task and the latest settled observation. Over-budget requests advance ahead
// of optional work waiting for one of the two workers.
type Backstage struct {
	mu      sync.Mutex
	lanes   map[backstageKey]*backstageLane
	queue   []*backstageTask
	running int
	workers sync.WaitGroup
	done    chan struct{}
}

func NewBackstage() *Backstage {
	return &Backstage{lanes: make(map[backstageKey]*backstageLane)}
}

func (b *Backstage) submit(ctx context.Context, key backstageKey, run string, settled, urgent bool, work func(context.Context) error) *backstageTask {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.done != nil || ctx.Err() != nil {
		return nil
	}
	lane := b.lanes[key]
	if lane != nil && !settled {
		lane.active.urgent = lane.active.urgent || urgent
		return lane.active
	}
	// Checkpoint work can outlive the originating call. Execute cancels its
	// jobs on failure and shutdown cancels all remaining work.
	owned, cancel := context.WithCancel(context.WithoutCancel(ctx))
	task := &backstageTask{key: key, run: run, ctx: owned, cancel: cancel, runJob: work, done: make(chan struct{}), urgent: urgent}
	if lane != nil {
		if lane.settled != nil {
			lane.settled.finish(context.Canceled)
		}
		lane.settled = task
		return task
	}
	b.lanes[key] = &backstageLane{active: task}
	b.queue = append(b.queue, task)
	b.dispatchLocked()
	return task
}

func (b *Backstage) dispatchLocked() {
	for b.done == nil && b.running < 2 && len(b.queue) > 0 {
		position := max(0, slices.IndexFunc(b.queue, func(task *backstageTask) bool { return task.urgent }))
		task := b.queue[position]
		b.queue = slices.Delete(b.queue, position, position+1)
		task.started = true
		b.running++
		b.workers.Add(1)
		go b.process(task)
	}
}

func (b *Backstage) process(task *backstageTask) {
	defer b.workers.Done()
	err := task.ctx.Err()
	if err == nil {
		err = task.runJob(task.ctx)
	}
	if err != nil && !errors.Is(err, records.ErrConflict) && !errors.Is(err, context.Canceled) {
		slog.Warn("automatic context compaction failed", "run_id", task.run)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running--
	task.finish(err)
	b.advanceLocked(task.key)
	b.dispatchLocked()
}

func (b *Backstage) advanceLocked(key backstageKey) {
	lane := b.lanes[key]
	if lane.settled == nil {
		delete(b.lanes, key)
		return
	}
	lane.active, lane.settled = lane.settled, nil
	b.queue = append(b.queue, lane.active)
}

func (b *Backstage) cancelRun(run string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, lane := range b.lanes {
		if lane.settled != nil && lane.settled.run == run {
			lane.settled.finish(context.Canceled)
			lane.settled = nil
		}
		task := lane.active
		if task.run != run {
			continue
		}
		if task.started {
			task.cancel()
			continue
		}
		b.queue = slices.DeleteFunc(b.queue, func(queued *backstageTask) bool { return queued == task })
		task.finish(context.Canceled)
		b.advanceLocked(key)
	}
	b.dispatchLocked()
}

func (b *Backstage) waitLane(ctx context.Context, key backstageKey) error {
	for {
		b.mu.Lock()
		lane := b.lanes[key]
		if lane == nil {
			b.mu.Unlock()
			return nil
		}
		task := lane.active
		b.mu.Unlock()
		_ = task.wait(ctx)
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (b *Backstage) stop() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.done == nil {
		b.done = make(chan struct{})
		for key, lane := range b.lanes {
			if lane.settled != nil {
				lane.settled.finish(context.Canceled)
				lane.settled = nil
			}
			if lane.active.started {
				lane.active.cancel()
			} else {
				lane.active.finish(context.Canceled)
				delete(b.lanes, key)
			}
		}
		b.queue = nil
		done := b.done
		go func() {
			b.workers.Wait()
			close(done)
		}()
	}
	return b.done
}
