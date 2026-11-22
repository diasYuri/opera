package execution

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/opera/internal/modules/accounting"
	"github.com/diasYuri/opera/internal/modules/context"
	"github.com/diasYuri/opera/internal/modules/records"
	"github.com/diasYuri/opera/internal/modules/summarizer"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/agent/compaction"
	ycontext "github.com/diasYuri/y/pkg/context"
	"github.com/diasYuri/y/pkg/policy"
	"github.com/diasYuri/y/pkg/providers"
	"sort"
	"strings"
	"time"
)

type SessionEntry struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"session_id"`
	Parent        string    `json:"parent,omitempty"`
	Origin        string    `json:"origin,omitempty"`
	OriginVersion uint64    `json:"origin_version,omitempty"`
	State         string    `json:"state"`
	Checkpoint    uint64    `json:"checkpoint"`
	Updated       time.Time `json:"updated"`
	Worker        bool      `json:"worker"`
}

func (s *Service) index(ctx context.Context, id string, m Metadata, result Result) error {
	if s.Records == nil {
		return nil
	}
	return s.Records.Put(ctx, "sessions", id, SessionEntry{ID: id, SessionID: m.SessionID, Parent: m.ParentID, Origin: m.Origin, OriginVersion: m.OriginVersion, State: result.State, Checkpoint: result.Checkpoint, Updated: time.Now().UTC(), Worker: m.Worker})
}
func (s *Service) Sessions(ctx context.Context, offset, limit int) ([]SessionEntry, error) {
	if offset < 0 || limit < 1 || limit > 1000 {
		return nil, errors.New("invalid pagination: offset >= 0; limit 1..1000")
	}
	docs, err := s.Records.List(ctx, "sessions")
	if err != nil {
		return nil, err
	}
	out := []SessionEntry{}
	for _, doc := range docs {
		var entry SessionEntry
		if err := json.Unmarshal(doc.Data, &entry); err != nil {
			return nil, err
		}
		if entry.State == "running" || entry.State == "queued" {
			s.activeMu.Lock()
			active := s.active[entry.ID] != nil
			s.activeMu.Unlock()
			if !active {
				entry.State = "interrupted"
			}
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	grouped := []SessionEntry{}
	seen := map[string]bool{}
	heads := map[string]string{}
	for _, entry := range out {
		key := entry.SessionID
		if key == "" {
			key = entry.ID
		}
		if seen[key] {
			continue
		}
		if s.SessionMemory != nil && entry.Parent == "" && entry.State == "completed" {
			head, known := heads[key]
			if !known {
				memory, err := s.SessionMemory.Read(ctx, key)
				if err != nil {
					return nil, err
				}
				head = memory.HeadRun
				heads[key] = head
			}
			if head != "" && head != entry.ID {
				metadata, err := s.Metadata(ctx, entry.ID)
				if err != nil {
					return nil, err
				}
				if metadata.ContinuedFrom != head {
					continue
				}
			}
		}
		seen[key] = true
		grouped = append(grouped, entry)
	}
	out = grouped
	if offset >= len(out) {
		return []SessionEntry{}, nil
	}
	return out[offset:min(len(out), offset+limit)], nil
}

type Inspection struct {
	Decisions       []policy.AuditEvent     `json:"decisions"`
	Usage           accounting.Summary      `json:"usage"`
	ArchivedOutputs []records.Document      `json:"archived_outputs"`
	Controls        []records.Document      `json:"controls"`
	Boundaries      []records.Document      `json:"boundaries"`
	SchemaVersion   int                     `json:"schema_version"`
	Metadata        Metadata                `json:"metadata"`
	Snapshot        agent.Snapshot          `json:"checkpoint"`
	Events          []agent.EventEnvelope   `json:"events"`
	Operations      []agent.OperationRecord `json:"operations"`
	Tasks           any                     `json:"tasks"`
	Children        []Child                 `json:"children"`
}

func (s *Service) Inspect(ctx context.Context, id string) (Inspection, error) {
	var out Inspection
	out.SchemaVersion = 1
	m, err := s.Metadata(ctx, id)
	if err != nil {
		return out, err
	}
	if m.Workspace != s.Workspace {
		return out, errors.New("session belongs to another workspace")
	}
	out.Metadata = m
	decisions, err := s.Records.List(ctx, "decisions")
	if err != nil {
		return out, err
	}
	for _, doc := range decisions {
		var event policy.AuditEvent
		if err := json.Unmarshal(doc.Data, &event); err != nil {
			return out, err
		}
		if event.Identity.RunID == id {
			out.Decisions = append(out.Decisions, event)
		}
	}
	sort.Slice(out.Decisions, func(i, j int) bool { return out.Decisions[i].Timestamp.Before(out.Decisions[j].Timestamp) })
	out.Usage, err = s.Usage(ctx, "run", id, "")
	if err != nil {
		return out, err
	}
	archived, err := s.Records.List(ctx, "tool-output")
	if err != nil {
		return out, err
	}
	for _, doc := range archived {
		if strings.HasPrefix(doc.Key, id+":") {
			out.ArchivedOutputs = append(out.ArchivedOutputs, doc)
		}
	}
	controls, e := s.Records.List(ctx, "controls")
	if e != nil {
		return out, e
	}
	for _, doc := range controls {
		var v struct{ RunID string }
		if e := json.Unmarshal(doc.Data, &v); e != nil {
			return out, e
		}
		if v.RunID == id {
			out.Controls = append(out.Controls, doc)
		}
	}
	boundaries, e := s.Records.List(ctx, "boundaries")
	if e != nil {
		return out, e
	}
	for _, doc := range boundaries {
		if strings.HasPrefix(doc.Key, id+":") {
			out.Boundaries = append(out.Boundaries, doc)
		}
	}
	out.Snapshot, err = s.Store.Load(ctx, id)
	if err != nil {
		return out, err
	}
	out.Events, err = s.Store.Read(ctx, id, 0)
	if err != nil {
		return out, err
	}
	out.Operations, err = s.Store.Operations(ctx, id)
	if err != nil {
		return out, err
	}
	if s.Work != nil {
		out.Tasks, err = s.Work.Tasks(ctx, id)
		if err != nil {
			return out, err
		}
		out.Children, err = s.Work.Children(ctx, id)
		if err != nil {
			return out, err
		}
	}
	if _, err := s.Records.Get(ctx, "sessions", id); errors.Is(err, sql.ErrNoRows) {
		if err := s.index(ctx, id, m, Result{State: string(out.Snapshot.Status), Checkpoint: out.Snapshot.Version}); err != nil {
			return out, err
		}
	} else if err != nil {
		return out, err
	}
	return out, nil
}
func (s *Service) Export(ctx context.Context, id string) (json.RawMessage, error) {
	value, err := s.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var redacted any
	if err := json.Unmarshal(policy.Redact(raw), &redacted); err != nil {
		return nil, err
	}
	scrub(redacted)
	return json.Marshal(redacted)
}
func (s *Service) Fork(ctx context.Context, id string, version uint64) (string, error) {
	if s.Active() {
		return "", errors.New("branch requires idle execution")
	}
	view, err := s.Inspect(ctx, id)
	if err != nil {
		return "", err
	}
	if version != 0 && version != view.Snapshot.Version {
		doc, e := s.Records.Get(ctx, "boundaries", fmt.Sprintf("%s:%d", id, version))
		if e != nil {
			return "", errors.New("checkpoint version is unavailable")
		}
		var boundary Boundary
		if err := json.Unmarshal(doc.Data, &boundary); err != nil {
			return "", err
		}
		if boundary.Snapshot.RunID != id || boundary.Snapshot.Version != version {
			return "", errors.New("historical checkpoint is inconsistent")
		}
		view.Snapshot = boundary.Snapshot
		view.Metadata = boundary.Metadata
		view.Tasks = boundary.Tasks
	}
	if view.Snapshot.Status != agent.RunCompleted {
		return "", errors.New("branch requires a completed checkpoint; resolve pending or uncertain operations first")
	}
	for _, op := range view.Operations {
		if op.State == agent.OperationPending {
			return "", agent.ErrOperationInDoubt
		}
	}
	child := rand.Text()
	state := agent.NewSnapshot(child, view.Snapshot.Agent)
	state.SessionID = child
	state.WorkspaceID = s.Workspace
	state.Status = agent.RunRecoverable
	state.Agent.RunID = child
	state.Agent.SessionID = child
	state.Agent.ContextRequest = ycontext.Request{}
	state.Agent.RunNonce = ""
	state.Agent.RuntimeViewInstanceID = ""
	state.Agent.State = agent.StateIdle
	state.Agent.PendingApproval = nil
	state.Agent.FollowUpQueue = nil
	state.Agent.PendingSteering = nil
	state.Agent.IdempotencyKey = ""
	state.Agent.PolicyIdentity = policy.Identity{}
	state.Agent.AuthorizationExpiresAt = time.Time{}
	state.Agent.PolicyVersion = ""
	state.Agent.RecoverableErrMsg = ""
	state.IdempotencyKey = ""
	m := view.Metadata
	m.SessionID = child
	m.ParentID = ""
	m.Origin = id
	m.OriginVersion = view.Snapshot.Version
	m.ContinuedFrom = ""
	m.Pending = nil
	if err := s.saveMetadata(ctx, child, m); err != nil {
		return "", err
	}
	if err := s.Store.Save(ctx, child, 0, state); err != nil {
		return "", err
	}
	if s.Work != nil {
		if err := s.Records.Put(ctx, "tasks", child, view.Tasks); err != nil {
			return "", err
		}
	}
	state, err = s.Store.Load(ctx, child)
	if err != nil {
		return "", err
	}
	if s.SessionMemory != nil {
		s.materializeContext(ctx, child)
	}
	return child, s.index(ctx, child, m, Result{State: "branched", Checkpoint: state.Version})
}

// ForkSummary creates a new session from a compacted copy of a completed run.
// The source checkpoint is left untouched.
func (s *Service) ForkSummary(ctx context.Context, id string) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := s.register(id, cancel, ctx, true); err != nil {
		return "", err
	}
	defer s.unregister(id, true)
	view, memory, compacted, err := s.summarizeCheckpoint(ctx, id)
	if err != nil {
		return "", err
	}
	if !compacted.Applied {
		return "", errors.New("session could not be summarized")
	}

	child := rand.Text()
	state := agent.NewSnapshot(child, view.Snapshot.Agent)
	state.SessionID = child
	state.WorkspaceID = s.Workspace
	state.Status = agent.RunRecoverable
	state.Agent.RunID = child
	state.Agent.SessionID = child
	state.Agent.Transcript = compacted.Messages
	state.Agent.ContextRequest = ycontext.Request{}
	state.Agent.RunNonce = ""
	state.Agent.RuntimeViewInstanceID = ""
	state.Agent.State = agent.StateIdle
	state.Agent.PendingApproval = nil
	state.Agent.FollowUpQueue = nil
	state.Agent.PendingSteering = nil
	state.Agent.IdempotencyKey = ""
	state.Agent.PolicyIdentity = policy.Identity{}
	state.Agent.AuthorizationExpiresAt = time.Time{}
	state.Agent.PolicyVersion = ""
	state.Agent.RecoverableErrMsg = ""
	state.IdempotencyKey = ""
	metadata := view.Metadata
	metadata.SessionID = child
	metadata.ParentID = ""
	metadata.Origin = id
	metadata.OriginVersion = view.Snapshot.Version
	metadata.ContinuedFrom = ""
	metadata.Pending = nil
	if err := s.saveMetadata(ctx, child, metadata); err != nil {
		return "", err
	}
	if err := s.Store.Save(ctx, child, 0, state); err != nil {
		return "", err
	}
	memory = summarizer.Context{Markdown: memory.Markdown}
	if err := s.SessionMemory.Commit(ctx, child, memory); err != nil {
		return "", err
	}
	s.materializeContext(ctx, child)
	state, err = s.Store.Load(ctx, child)
	if err != nil {
		return "", err
	}
	return child, s.index(ctx, child, metadata, Result{State: "branched", Checkpoint: state.Version})
}

func scrub(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if text, ok := child.(string); ok {
				v[key] = contextdata.RedactText(text)
			} else {
				scrub(child)
			}
		}
	case []any:
		for i, child := range v {
			if text, ok := child.(string); ok {
				v[i] = contextdata.RedactText(text)
			} else {
				scrub(child)
			}
		}
	}
}

type limitedSummarizer struct {
	service *Service
	id      string
	base    compaction.Summarizer
}

func (p limitedSummarizer) Stream(ctx context.Context, request providers.StreamRequest) (providers.EventStream, error) {
	request.Options.RunID = p.id
	request.Options.TurnID = "compaction"
	estimated := providers.EstimateTokens(request.Context)
	if estimated > int64(p.service.MaxTokens) {
		return nil, errors.New("compaction input exceeds context budget")
	}
	if p.service.Work != nil {
		if err := p.service.Work.Charge(ctx, p.id, estimated+request.Options.MaxTokens); err != nil {
			return nil, err
		}
	}
	return p.base.Stream(ctx, request)
}

func (s *Service) CheckReconfiguration(ctx context.Context, exceptRun string) error {
	s.activeMu.Lock()
	count := len(s.active)
	_, own := s.active[exceptRun]
	s.activeMu.Unlock()
	if count != 0 && (count > 1 || !own) {
		return errors.New("reconfiguration requires an idle execution safe point")
	}
	documents, err := s.Records.List(ctx, "sessions")
	if err != nil {
		return err
	}
	for _, doc := range documents {
		if doc.Key == exceptRun {
			continue
		}
		var entry SessionEntry
		if err := json.Unmarshal(doc.Data, &entry); err != nil {
			return err
		}
		if entry.State == "waiting_approval" || entry.State == "awaiting_approval" || entry.State == "waiting_children" {
			return errors.New("resolve pending approvals before changing extension generations")
		}
	}
	return nil
}
