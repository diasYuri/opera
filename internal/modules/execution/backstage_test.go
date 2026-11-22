package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackstagePrioritizesAnOverBudgetLane(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	backstage := NewBackstage()
	defer func() { <-backstage.stop() }()
	block := make(chan struct{})
	started := make(chan string, 4)
	submit := func(name string, urgent bool, blocking bool) *backstageTask {
		return backstage.submit(ctx, backstageKey{session: name}, name, false, urgent, func(ctx context.Context) error {
			started <- name
			if blocking {
				select {
				case <-block:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}
	submit("first", false, true)
	submit("second", false, true)
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("workers did not start")
		}
	}
	optional := submit("optional", false, false)
	urgent := submit("urgent", false, false)
	if duplicate := submit("urgent", true, false); duplicate != urgent {
		t.Fatal("over-budget request should promote the queued work in its lane")
	}
	block <- struct{}{}
	select {
	case name := <-started:
		if name != "urgent" {
			t.Fatalf("started %q before urgent work", name)
		}
	case <-ctx.Done():
		t.Fatal("no queued work started")
	}
	if err := optional.wait(ctx); err != nil {
		t.Fatal(err)
	}
	close(block)
}

func TestBackstageCoalescesCompletedCheckpointObservations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	backstage := NewBackstage()
	defer func() { <-backstage.stop() }()
	key := backstageKey{session: "conversation"}
	block := make(chan struct{})
	backstage.submit(ctx, key, "run", false, false, func(ctx context.Context) error {
		select {
		case <-block:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	obsolete := backstage.submit(ctx, key, "old", true, false, func(context.Context) error {
		return errors.New("obsolete checkpoint ran")
	})
	latest := backstage.submit(ctx, key, "new", true, false, func(context.Context) error {
		return nil
	})
	if err := obsolete.wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected coalesced observation to be canceled: %v", err)
	}
	close(block)
	if err := latest.wait(ctx); err != nil {
		t.Fatal(err)
	}
}
