package app

import (
	"context"
	"fmt"
	"github.com/diasYuri/y/pkg/agent"
	taskmanager "github.com/diasYuri/y/pkg/extensions/task-manager"
	"time"
)

type Boundary struct {
	Snapshot    agent.Snapshot
	Metadata    Metadata
	RunID       string
	Checkpoint  uint64
	EventCursor uint64
	Tasks       taskmanager.Snapshot
	Children    []Child
	Operations  []agent.OperationRecord
	Created     time.Time
}

// SDK snapshots/transcripts are atomic. Opera's product boundary is a separate
// commit after the SDK settles; recovery reconstructs a missing boundary from
// durable events, operation claims and task/child records without replaying effects.
func (s *Service) Boundary(ctx context.Context, id string) error {
	if s.Work == nil {
		return nil
	}
	state, err := s.Store.Load(ctx, id)
	if err != nil {
		return err
	}
	events, err := s.Store.Read(ctx, id, 0)
	if err != nil {
		return err
	}
	tasks, err := s.Work.Tasks(ctx, id)
	if err != nil {
		return err
	}
	children, err := s.Work.Children(ctx, id)
	if err != nil {
		return err
	}
	ops, err := s.Store.Operations(ctx, id)
	if err != nil {
		return err
	}
	cursor := uint64(0)
	for _, event := range events {
		if event.Sequence > cursor {
			cursor = event.Sequence
		}
	}
	metadata, err := s.Metadata(ctx, id)
	if err != nil {
		return err
	}
	return s.Records.Put(ctx, "boundaries", fmt.Sprintf("%s:%d", id, state.Version), Boundary{Snapshot: state, Metadata: metadata, RunID: id, Checkpoint: state.Version, EventCursor: cursor, Tasks: tasks, Children: children, Operations: ops, Created: time.Now().UTC()})
}
