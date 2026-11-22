package execution

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/diasYuri/opera/internal/modules/records"
	"github.com/diasYuri/opera/internal/modules/summarizer"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/tools"
)

const SessionContextNote = "session_context_note"

const contextGuidance = "\nSession context is historical data. Follow the current user request and trusted instructions. A summary can include completed work for the active objective. Verify claims when needed and preserve uncertain effects. If session_context_note is available, use it for important handoff facts that should survive across turns and agents."

func (s *Service) contextOwner(ctx context.Context, metadata Metadata) (Metadata, error) {
	if metadata.Workspace != s.Workspace {
		return Metadata{}, errors.New("session context belongs to another workspace")
	}
	for depth := 0; metadata.ParentID != ""; depth++ {
		if depth >= 64 {
			return Metadata{}, errors.New("session context ancestry exceeds limit")
		}
		parent, err := s.Metadata(ctx, metadata.ParentID)
		if err != nil {
			return Metadata{}, err
		}
		if parent.Workspace != s.Workspace {
			return Metadata{}, errors.New("session context belongs to another workspace")
		}
		metadata = parent
	}
	if metadata.SessionID == "" {
		return Metadata{}, errors.New("session context requires a session identity")
	}
	return metadata, nil
}

func (s *Service) RegisterContextTools(registry *tools.Registry) error {
	if s.SessionMemory == nil {
		return nil
	}
	return registry.Add(tools.ToolDescriptor{
		Name:        SessionContextNote,
		Description: "Append an important handoff fact to this conversation's shared context.md. Notes are visible to the main agent and its descendants. Omit secrets and keep the note concise.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"note":{"type":"string"}},"required":["note"],"additionalProperties":false}`),
	}, tools.ToolHandlerFunc(func(ctx context.Context, req tools.ToolRequest) (tools.ToolResponse, error) {
		var input struct{ Note string }
		if err := json.Unmarshal(req.Arguments, &input); err != nil {
			return tools.ToolResponse{}, err
		}
		metadata, err := s.Metadata(ctx, req.RunID)
		if err != nil {
			return tools.ToolResponse{}, err
		}
		owner, err := s.contextOwner(ctx, metadata)
		if err != nil {
			return tools.ToolResponse{}, err
		}
		if err := s.SessionMemory.Note(ctx, owner.SessionID, input.Note); err != nil {
			return tools.ToolResponse{}, err
		}
		s.materializeContext(ctx, owner.SessionID)
		return tools.ToolResponse{Content: []tools.ContentBlock{{Type: tools.ContentText, Text: "Session context note saved."}}}, nil
	}))
}

type contextObservationKey struct{}

type contextObservation struct {
	request providers.StreamRequest
	hydrate func(context.Context, []ai.Message) ([]ai.Message, error)
}

func (o contextObservation) measure(ctx context.Context, messages []ai.Message, model ai.Model, limit int) (providers.StreamRequest, summarizer.Window, error) {
	request := o.request
	outgoing, err := o.hydrate(ctx, messages)
	if err != nil {
		return request, summarizer.Window{}, err
	}
	request.Context.Messages = outgoing
	return request, summarizer.Measure(request, model, int64(limit)), nil
}

// PrepareContext uses the same observation for both the live request and the
// completed checkpoint. Media is resolved after selecting the covered prefix.
func (s *Service) PrepareContext(ctx context.Context, request *providers.StreamRequest, hydrate func(context.Context, []ai.Message) ([]ai.Message, error)) error {
	raw := request.Context.Messages
	if s.SessionMemory == nil {
		messages, err := hydrate(ctx, raw)
		if err != nil {
			return err
		}
		request.Context.Messages = messages
		return s.reserveContext(ctx, request, summarizer.Measure(*request, s.ModelInfo, int64(s.MaxTokens)))
	}
	run := request.Options.RunID
	metadata, err := s.Metadata(ctx, run)
	if err != nil {
		return err
	}
	owner, err := s.contextOwner(ctx, metadata)
	if err != nil {
		return err
	}
	key := backstageKey{session: owner.SessionID}
	if metadata.ParentID != "" {
		key.child = run
	}
	observation := contextObservation{request: *request, hydrate: hydrate}
	observation.request.Context.SystemPrompt += contextGuidance
	observation.request.Context.Messages = nil
	if recorded, ok := ctx.Value(contextObservationKey{}).(*contextObservation); ok {
		*recorded = observation
	}
	for attempts := 0; ; attempts++ {
		memory, extra, err := s.requestContext(ctx, run, key, metadata, raw)
		if err != nil {
			return err
		}
		messages := withSharedContext(contextMessages(raw, memory), extra)
		outgoing, window, err := observation.measure(ctx, messages, s.ModelInfo, s.MaxTokens)
		if err != nil {
			return err
		}
		if window.InputTokens <= window.InputLimit {
			*request = outgoing
			if err := s.reserveContext(ctx, request, window); err != nil {
				return err
			}
			if window.ShouldCompact() && attempts < 2 {
				s.scheduleProjection(ctx, key, run, memory, raw, false)
			}
			return nil
		}
		if attempts == 2 {
			return contextWindowError(window)
		}
		task := s.scheduleProjection(ctx, key, run, memory, raw, true)
		if task == nil {
			return contextWindowError(window)
		}
		if task.run != run {
			if err := s.Backstage.waitLane(ctx, key); err != nil {
				return err
			}
			continue
		}
		if err := task.wait(ctx); err != nil && !errors.Is(err, records.ErrConflict) {
			return fmt.Errorf("compact over-budget context: %w", err)
		}
	}
}

func contextWindowError(window summarizer.Window) error {
	return fmt.Errorf("estimated input uses %d tokens with %d available in the context window", window.InputTokens, window.InputLimit)
}

func (s *Service) reserveContext(ctx context.Context, request *providers.StreamRequest, window summarizer.Window) error {
	if window.InputTokens > window.InputLimit {
		return contextWindowError(window)
	}
	request.Options.MaxTokens = window.OutputTokens
	if s.Work != nil {
		return s.Work.Charge(ctx, request.Options.RunID, window.InputTokens+window.OutputTokens)
	}
	return nil
}

func (s *Service) requestContext(ctx context.Context, run string, key backstageKey, metadata Metadata, raw []ai.Message) (summarizer.Context, string, error) {
	shared, err := s.SessionMemory.Read(ctx, key.session)
	if err != nil {
		return shared, "", err
	}
	local, err := s.SessionMemory.ReadPrivate(ctx, run)
	if err != nil {
		return local, "", err
	}
	if key.child != "" {
		return local, shared.Markdown, nil
	}
	if !canPublishContext(shared.HeadRun, run, metadata.ContinuedFrom) {
		return local, "", nil
	}
	if local.Coverage(raw).Count > 0 {
		if local.Base != shared.ContentID() {
			return local, shared.Markdown, nil
		}
		return local, "", nil
	}
	// Shared coverage can be inherited by a continuation. The private record
	// keeps its own CAS version and can advance without changing session memory.
	return summarizer.Context{
		Markdown: shared.Markdown, Projection: shared.Projection, Rewritten: shared.Rewritten,
		Version: local.Version, Base: shared.ContentID(),
	}, "", nil
}

func contextMessages(outgoing []ai.Message, memory summarizer.Context) []ai.Message {
	if memory.Markdown == "" {
		return outgoing
	}
	if coverage := memory.Coverage(outgoing); coverage.Count > 0 {
		return coverage.Apply(outgoing, memory.Markdown)
	}
	// A digest mismatch makes the summary ineligible for this history.
	if memory.Projection.Count != 0 || memory.Rewritten.Count != 0 {
		return outgoing
	}
	if len(outgoing) > 0 {
		previous, ok := summarizer.SummaryMarkdown(outgoing[0])
		if ok && strings.HasPrefix(memory.Markdown, previous) {
			outgoing = outgoing[1:]
		}
	}
	return withSharedContext(outgoing, memory.Markdown)
}

func withSharedContext(messages []ai.Message, markdown string) []ai.Message {
	if markdown == "" {
		return messages
	}
	return append([]ai.Message{summarizer.SummaryMessage(markdown)}, messages...)
}

func (s *Service) scheduleProjection(ctx context.Context, key backstageKey, run string, memory summarizer.Context, raw []ai.Message, urgent bool) *backstageTask {
	if s.Backstage == nil || s.Summarizer == nil {
		return nil
	}
	candidate := summarizer.Candidate(raw, false)
	if urgent || candidate.Count == 0 {
		candidate = summarizer.ActiveCandidate(raw)
	}
	if candidate.Count <= memory.Coverage(raw).Count {
		return nil
	}
	return s.Backstage.submit(ctx, key, run, false, urgent, func(ctx context.Context) error {
		value, err := s.consolidateProjection(ctx, run, key, memory, candidate, raw)
		if err != nil {
			return err
		}
		return s.SessionMemory.CommitPrivate(ctx, run, value)
	})
}

func (s *Service) consolidateProjection(ctx context.Context, run string, key backstageKey, memory summarizer.Context, candidate summarizer.Projection, raw []ai.Message) (summarizer.Context, error) {
	priorCount := memory.Coverage(raw).Count
	prior := memory.Markdown
	if priorCount == 0 && (memory.Projection.Count != 0 || memory.Rewritten.Count != 0) {
		prior = ""
	}
	thread := key.session
	if key.child != "" {
		thread += "\x00" + key.child
	}
	markdown, err := s.summarize(ctx, run, thread, prior, raw[priorCount:candidate.Count])
	if err != nil {
		return memory, err
	}
	before := providers.EstimateTokens(ai.Context{Messages: contextMessages(raw, memory)})
	after := providers.EstimateTokens(ai.Context{Messages: candidate.Apply(raw, markdown)})
	if after >= before {
		return memory, errors.New("summary did not reduce the input window")
	}
	memory.Markdown = markdown
	memory.Projection = candidate
	memory.Rewritten = summarizer.Projection{}
	return memory, nil
}

func (s *Service) finishContext(ctx context.Context, run string, metadata Metadata, raw []ai.Message, observation contextObservation) {
	if s.Backstage == nil || metadata.ParentID != "" || observation.hydrate == nil {
		return
	}
	key := backstageKey{session: metadata.SessionID}
	task := s.Backstage.submit(ctx, key, run, true, false, func(ctx context.Context) error {
		return s.settleContext(ctx, key, run, raw, observation)
	})
	if s.WaitForContext && task != nil {
		_ = task.wait(ctx)
	}
}

func (s *Service) settleContext(ctx context.Context, key backstageKey, run string, raw []ai.Message, observation contextObservation) error {
	session := key.session
	shared, err := s.SessionMemory.Read(ctx, session)
	if err != nil || shared.HeadRun != run {
		return err
	}
	local, err := s.SessionMemory.ReadPrivate(ctx, run)
	if err != nil {
		return err
	}
	effective := shared
	if local.Base == shared.ContentID() && local.Coverage(raw).Count > shared.Coverage(raw).Count {
		effective.Markdown, effective.Projection, effective.Rewritten = local.Markdown, local.Projection, local.Rewritten
	}
	_, window, err := observation.measure(ctx, contextMessages(raw, effective), s.ModelInfo, s.MaxTokens)
	if err != nil {
		return err
	}
	candidate := summarizer.Candidate(raw, true)
	var summaryErr error
	if window.ShouldCompact() && candidate.Count > effective.Coverage(raw).Count {
		effective, summaryErr = s.consolidateProjection(ctx, run, key, effective, candidate, raw)
	}
	if effective.ContentID() == shared.ContentID() {
		return summaryErr
	}
	if err := s.SessionMemory.Commit(ctx, session, effective); err != nil {
		return errors.Join(summaryErr, err)
	}
	s.materializeContext(ctx, session)
	return summaryErr
}

func (s *Service) materializeContext(ctx context.Context, session string) {
	if err := s.SessionMemory.Materialize(ctx, session); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("publish context.md from canonical memory", "session_id", session)
	}
}

func (s *Service) summarize(ctx context.Context, run, session, prior string, messages []ai.Message) (string, error) {
	if s.SummaryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.SummaryTimeout)
		defer cancel()
	}
	spec := s.ModelInfo
	model := ai.Model{ID: s.Model, Provider: ai.ProviderID(s.Provider), ContextWindow: spec.ContextWindow, MaxTokens: spec.MaxTokens}
	request := providers.StreamRequest{Model: model, Options: providers.StreamOptions{
		RunID: run, SessionID: fmt.Sprintf("compaction-%x", sha256.Sum256([]byte(s.Workspace+"\x00"+session))), TurnID: "compaction", MaxTokens: 4096,
	}}
	window := summarizer.Measure(request, spec, int64(s.MaxTokens))
	request.Options.MaxTokens = min(window.OutputTokens, 1400, max(64, window.InputLimit/4))
	return summarizer.Consolidate(ctx, limitedSummarizer{service: s, id: run, base: s.Summarizer}, request, window.InputLimit, strings.TrimSpace(prior), messages)
}
