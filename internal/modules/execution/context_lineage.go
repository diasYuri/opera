package execution

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/diasYuri/opera/internal/modules/records"
	"github.com/diasYuri/y/pkg/agent"
)

var errContextLineage = errors.New("checkpoint is outside the current session history; create a branch to work from it")

// initializeSessionHead adopts the latest completed run from the existing
// session index for sessions created before Backstage tracked lineage.
func (s *Service) initializeSessionHead(ctx context.Context, session string) (string, error) {
	for range 4 {
		memory, err := s.SessionMemory.Read(ctx, session)
		if err != nil || memory.HeadRun != "" {
			return memory.HeadRun, err
		}
		documents, err := s.Records.List(ctx, "sessions")
		if err != nil {
			return "", err
		}
		var latest string
		var completedAt time.Time
		for _, document := range documents {
			var entry SessionEntry
			if err := json.Unmarshal(document.Data, &entry); err != nil {
				return "", err
			}
			if entry.SessionID != session || entry.Parent != "" || entry.State != "completed" {
				continue
			}
			// Replaying a run touches its session index timestamp. Use the
			// committed SDK checkpoint to distinguish historical replays.
			checkpoint, err := s.Store.Load(ctx, entry.ID)
			if errors.Is(err, agent.ErrStateNotFound) {
				continue
			}
			if err != nil {
				return "", err
			}
			if checkpoint.Status == agent.RunCompleted && checkpoint.UpdatedAt.After(completedAt) {
				latest, completedAt = entry.ID, checkpoint.UpdatedAt
			}
		}
		if latest == "" {
			return "", nil
		}
		memory.HeadRun = latest
		if err := s.SessionMemory.Commit(ctx, session, memory); !errors.Is(err, records.ErrConflict) {
			return memory.HeadRun, err
		}
	}
	return "", records.ErrConflict
}

// useCompletedHead repairs a crash between indexing a completed run and
// advancing its head. The caller has already verified the Y checkpoint.
func (s *Service) useCompletedHead(ctx context.Context, run string, metadata Metadata) error {
	head, err := s.initializeSessionHead(ctx, metadata.SessionID)
	if err != nil || head == run {
		return err
	}
	if !canPublishContext(head, run, metadata.ContinuedFrom) {
		return errContextLineage
	}
	return s.advanceSessionHead(ctx, metadata.SessionID, run, metadata.ContinuedFrom)
}

func canPublishContext(head, run, predecessor string) bool {
	return head == "" || head == run || head == predecessor
}

func (s *Service) advanceSessionHead(ctx context.Context, session, run, predecessor string) error {
	for range 4 {
		memory, err := s.SessionMemory.Read(ctx, session)
		if err != nil || memory.HeadRun == run {
			return err
		}
		if !canPublishContext(memory.HeadRun, run, predecessor) {
			return errContextLineage
		}
		memory.HeadRun = run
		if err := s.SessionMemory.Commit(ctx, session, memory); !errors.Is(err, records.ErrConflict) {
			return err
		}
	}
	return records.ErrConflict
}
