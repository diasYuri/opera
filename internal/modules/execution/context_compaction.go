package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/diasYuri/opera/internal/modules/records"
	"github.com/diasYuri/opera/internal/modules/summarizer"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/agent/compaction"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
)

const compactionArchiveBucket = "compactions"

type compactionArchive struct {
	Before     agent.Snapshot
	Compaction compaction.Result
	State      string
	// The canonical commit and rewritten checkpoint each carry evidence of
	// their result, so a retry can resume across either write boundary.
	Session       string
	MemoryVersion uint64
}

func (s *Service) Compact(ctx context.Context, id string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := s.register(id, cancel, ctx, true); err != nil {
		return err
	}
	defer s.unregister(id, true)
	view, err := s.checkpointForCompaction(ctx, id)
	if err != nil {
		return err
	}
	session := view.Metadata.SessionID
	if err := s.waitForCompactionLane(ctx, session); err != nil {
		return err
	}
	raw := view.Snapshot.Agent.Transcript
	if len(raw) == 1 && view.Snapshot.Version > 0 {
		key := fmt.Sprintf("%s:%d", id, view.Snapshot.Version-1)
		archive, version, err := s.readCompactionArchive(ctx, key)
		if err != nil {
			return err
		}
		if archive.Session == session && archive.matchesRewrite(view.Snapshot) {
			if err := s.finishCompactionArchive(ctx, key, archive, version); err != nil {
				return err
			}
			s.materializeContext(ctx, session)
			return nil
		}
	}
	key := fmt.Sprintf("%s:%d", id, view.Snapshot.Version)
	archive, version, err := s.readCompactionArchive(ctx, key)
	if err != nil {
		return err
	}
	memory, err := s.SessionMemory.Read(ctx, session)
	if err != nil {
		return err
	}
	if archive.State == "published" || memory.CommitID == key {
		return s.applyArchivedCompaction(ctx, key, archive, version)
	}
	if err := s.useCompletedHead(ctx, id, view.Metadata); err != nil {
		return err
	}
	memory, err = s.SessionMemory.Read(ctx, session)
	if err != nil {
		return err
	}
	if memory.HeadRun != id {
		return errContextLineage
	}
	if memory.CommitID == key {
		archive, version, err = s.readCompactionArchive(ctx, key)
		if err != nil {
			return err
		}
		return s.applyArchivedCompaction(ctx, key, archive, version)
	}
	if err := s.completePriorCompaction(ctx, memory.CommitID); err != nil {
		return err
	}
	if archive.Session != session || archive.MemoryVersion != memory.Version {
		compacted, err := s.summarizeSnapshot(ctx, view, memory)
		if err != nil {
			return err
		}
		if !compacted.Applied {
			return errors.New("checkpoint has no new conversation to compact")
		}
		archive = compactionArchive{
			Before: view.Snapshot, Compaction: compacted, State: "prepared",
			Session: session, MemoryVersion: memory.Version,
		}
		if err := s.SessionMemory.Store.CAS(ctx, compactionArchiveBucket, key, version, archive); err != nil {
			return fmt.Errorf("prepare compaction archive: %w", err)
		}
		version++
	}
	markdown, err := archive.markdown()
	if err != nil {
		return err
	}
	memory.Markdown = markdown
	memory.CommitID = key
	memory.Projection = summarizer.Candidate(raw, true)
	memory.Rewritten = summarizer.RewrittenPrefix(archive.Compaction.Messages[0])
	if err := s.SessionMemory.Commit(ctx, session, memory); err != nil {
		return fmt.Errorf("publish canonical compaction: %w", err)
	}
	return s.applyArchivedCompaction(ctx, key, archive, version)
}

// Complete the operation identified by the canonical record before replacing
// its publication marker. This also repairs journals left by another process.
func (s *Service) completePriorCompaction(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	archive, version, err := s.readCompactionArchive(ctx, key)
	if err != nil || archive.State == "committed" {
		return err
	}
	return s.applyArchivedCompaction(ctx, key, archive, version)
}

func (s *Service) applyArchivedCompaction(ctx context.Context, key string, archive compactionArchive, version uint64) error {
	if _, err := archive.markdown(); err != nil {
		return err
	}
	if archive.State == "prepared" {
		archive.State = "published"
		if err := s.SessionMemory.Store.CAS(ctx, compactionArchiveBucket, key, version, archive); err != nil {
			return fmt.Errorf("canonical compaction published; retry journal update: %w", err)
		}
		version++
	}
	id := archive.Before.RunID
	state, err := s.Store.Load(ctx, id)
	if err != nil {
		return err
	}
	if !archive.matchesRewrite(state) {
		if state.Version != archive.Before.Version {
			return agent.ErrVersionConflict
		}
		state.Agent.Transcript = archive.Compaction.Messages
		if err := s.Store.Save(ctx, id, state.Version, state); err != nil {
			if !errors.Is(err, agent.ErrVersionConflict) {
				return fmt.Errorf("canonical compaction published; retry checkpoint rewrite: %w", err)
			}
			latest, loadErr := s.Store.Load(ctx, id)
			if loadErr != nil || !archive.matchesRewrite(latest) {
				return fmt.Errorf("resolve compacted checkpoint: %w", errors.Join(err, loadErr))
			}
		}
	}
	if err := s.finishCompactionArchive(ctx, key, archive, version); err != nil {
		return err
	}
	s.materializeContext(ctx, archive.Session)
	return nil
}

func (s *Service) readCompactionArchive(ctx context.Context, key string) (compactionArchive, uint64, error) {
	var archive compactionArchive
	doc, err := s.Records.Get(ctx, compactionArchiveBucket, key)
	if errors.Is(err, sql.ErrNoRows) {
		return archive, 0, nil
	}
	if err == nil {
		err = json.Unmarshal(doc.Data, &archive)
	}
	return archive, doc.Version, err
}

func (a compactionArchive) markdown() (string, error) {
	if len(a.Compaction.Messages) == 1 {
		if markdown, ok := summarizer.SummaryMarkdown(a.Compaction.Messages[0]); ok {
			return markdown, nil
		}
	}
	return "", errors.New("compaction archive has no valid summary")
}

func (a compactionArchive) matchesRewrite(snapshot agent.Snapshot) bool {
	return snapshot.Version == a.Before.Version+1 && len(snapshot.Agent.Transcript) == 1 &&
		len(a.Compaction.Messages) == 1 && summarizer.RewrittenPrefix(a.Compaction.Messages[0]).Matches(snapshot.Agent.Transcript)
}

func (s *Service) finishCompactionArchive(ctx context.Context, key string, archive compactionArchive, version uint64) error {
	if archive.State == "committed" {
		return nil
	}
	archive.State = "committed"
	err := s.SessionMemory.Store.CAS(ctx, compactionArchiveBucket, key, version, archive)
	if errors.Is(err, records.ErrConflict) {
		current, _, readErr := s.readCompactionArchive(ctx, key)
		if readErr == nil && current.State == "committed" {
			return nil
		}
		err = errors.Join(err, readErr)
	}
	if err != nil {
		return fmt.Errorf("checkpoint compacted; retry archive finalization: %w", err)
	}
	return nil
}

func (s *Service) checkpointForCompaction(ctx context.Context, id string) (Inspection, error) {
	view, err := s.Inspect(ctx, id)
	if err != nil {
		return view, err
	}
	if view.Snapshot.Status != agent.RunCompleted || view.Metadata.ParentID != "" {
		return view, errors.New("compaction requires a completed main conversation checkpoint")
	}
	for _, op := range view.Operations {
		if op.State == agent.OperationPending {
			return view, agent.ErrOperationInDoubt
		}
	}
	if s.Summarizer == nil || s.SessionMemory == nil {
		return view, errors.New("summarizer unavailable")
	}
	return view, nil
}

func (s *Service) waitForCompactionLane(ctx context.Context, session string) error {
	if s.Backstage == nil {
		return nil
	}
	return s.Backstage.waitLane(ctx, backstageKey{session: session})
}

// A summary fork may use compatible coverage from the source checkpoint.
func (s *Service) summarizeCheckpoint(ctx context.Context, id string) (Inspection, summarizer.Context, compaction.Result, error) {
	view, err := s.checkpointForCompaction(ctx, id)
	if err != nil {
		return view, summarizer.Context{}, compaction.Result{}, err
	}
	session := view.Metadata.SessionID
	if err := s.waitForCompactionLane(ctx, session); err != nil {
		return view, summarizer.Context{}, compaction.Result{}, err
	}
	memory, err := s.SessionMemory.Read(ctx, session)
	if err != nil {
		return view, memory, compaction.Result{}, err
	}
	if memory.HeadRun != "" && memory.HeadRun != id && memory.Coverage(view.Snapshot.Agent.Transcript).Count == 0 {
		memory = summarizer.Context{}
	}
	compacted, err := s.summarizeSnapshot(ctx, view, memory)
	if compacted.Applied {
		memory.Markdown, _ = summarizer.SummaryMarkdown(compacted.Messages[0])
	}
	return view, memory, compacted, err
}

func (s *Service) summarizeSnapshot(ctx context.Context, view Inspection, memory summarizer.Context) (compaction.Result, error) {
	raw := view.Snapshot.Agent.Transcript
	if len(raw) < 2 {
		if len(raw) == 1 {
			if _, ok := summarizer.SummaryMarkdown(raw[0]); ok {
				return compaction.Result{Applied: true, Messages: raw}, nil
			}
		}
		return compaction.Result{}, nil
	}
	if !summarizer.Settled(raw) {
		return compaction.Result{}, errors.New("compaction requires settled tool calls")
	}
	offset := memory.Coverage(raw).Count
	markdown := memory.Markdown
	if offset == 0 && (memory.Projection.Count != 0 || memory.Rewritten.Count != 0) {
		markdown = ""
	}
	if offset < len(raw) {
		var err error
		markdown, err = s.summarize(ctx, view.Snapshot.RunID, view.Metadata.SessionID, markdown, raw[offset:])
		if err != nil {
			return compaction.Result{}, err
		}
	}
	messages := []ai.Message{summarizer.SummaryMessage(markdown)}
	meta := compaction.Metadata{
		Reason: compaction.ReasonManual, Boundary: len(raw),
		TokensBefore: providers.EstimateTokens(ai.Context{Messages: raw}),
		TokensAfter:  providers.EstimateTokens(ai.Context{Messages: messages}),
	}
	for _, message := range raw {
		if message.ID != "" {
			meta.ArchivedMessageIDs = append(meta.ArchivedMessageIDs, message.ID)
		}
		for _, call := range message.ToolCalls {
			meta.ToolCallIDs = append(meta.ToolCallIDs, call.ID)
		}
		if message.ToolResult != nil {
			meta.ToolResultCallIDs = append(meta.ToolResultCallIDs, message.ToolResult.ToolCallID)
		}
	}
	return compaction.Result{Applied: true, Messages: messages, ArchivedMessages: raw, Metadata: meta}, nil
}
