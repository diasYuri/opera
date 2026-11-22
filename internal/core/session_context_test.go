package core_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/opera/internal/modules/summarizer"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/tools"
)

func TestAutomaticSessionContextSurvivesRestart(t *testing.T) {
	cfg := configuration(t)
	ctx := context.Background()
	verbose := strings.Repeat("verbose result to archive in the durable transcript. ", 3200)
	var attachment bytes.Buffer
	if err := png.Encode(&attachment, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(cfg.Paths.Workspace, "sample.png")
	if err := os.WriteFile(imagePath, attachment.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	firstProvider := agenttest.NewScriptProvider(t, agenttest.TextTurn(verbose), agenttest.TextTurn(layeredSummary))
	runtime := openContextRuntime(t, ctx, cfg, imageContextProvider{namedProvider{firstProvider}}, core.Options{Profile: core.ProfileHeadless})
	first, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "inspect the code", ImagePaths: []string{"sample.png"}}, nil)
	if err != nil || first.Text != verbose {
		t.Fatalf("main response changed: %q %v", first.State, err)
	}
	requests := firstProvider.Requests()
	if len(requests) != 2 || len(requests[1].Context.Tools) != 0 ||
		requests[1].Options.SessionID == requests[0].Options.SessionID ||
		!strings.HasPrefix(requests[1].Options.SessionID, "compaction-") {
		t.Fatal("summary did not use an isolated inference thread")
	}
	memory, err := runtime.Service.SessionMemory.Read(ctx, first.SessionID)
	if err != nil || memory.Projection.Count != 2 {
		t.Fatalf("missing durable projection: %+v %v", memory, err)
	}
	files, err := filepath.Glob(filepath.Join(cfg.Paths.Data, "sessions", "*", "context.md"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one context.md: %v %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil || !strings.Contains(string(data), layeredSummary) {
		t.Fatalf("context.md missing layered summary: %q %v", data, err)
	}
	raw, err := runtime.Service.Store.Load(ctx, first.ID)
	if err != nil || raw.Agent.Transcript[1].Content[0].Text != verbose {
		t.Fatal("automatic compaction changed the durable transcript", err)
	}
	closeRuntime(t, runtime)
	if err := os.Remove(files[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(imagePath); err != nil {
		t.Fatal(err)
	}

	secondProvider := agenttest.NewScriptProvider(t, agenttest.TextTurn("continued"))
	runtime = openContextRuntime(t, ctx, cfg, namedProvider{secondProvider}, core.Options{Profile: core.ProfileHeadless})
	defer closeRuntime(t, runtime)
	second, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "verify the result", ContinueFrom: first.ID}, nil)
	if err != nil || second.SessionID != first.SessionID || second.Text != "continued" {
		t.Fatalf("continuation: %+v %v", second, err)
	}
	if restored, err := os.ReadFile(files[0]); err != nil || string(restored) != string(data) {
		t.Fatalf("context.md was not restored from the durable record: %v", err)
	}
	messages := secondProvider.Requests()[0].Context.Messages
	if len(messages) != 2 || !strings.Contains(messages[0].Content[0].Text, layeredSummary) || messages[1].Content[0].Text != "verify the result" {
		t.Fatal("continuation did not use the durable projection")
	}
}

type imageContextProvider struct{ namedProvider }

func (imageContextProvider) Capabilities(string) providers.Capabilities {
	return providers.Capabilities{Streaming: true, Vision: true}
}

type blockedSummaryProvider struct {
	namedProvider
	started  chan struct{}
	finished chan struct{}
}

func (p blockedSummaryProvider) Stream(ctx context.Context, request providers.StreamRequest) (providers.EventStream, error) {
	if !strings.HasPrefix(request.Options.SessionID, "compaction-") {
		return p.namedProvider.Stream(ctx, request)
	}
	close(p.started)
	defer close(p.finished)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestInteractiveSessionDoesNotWaitForBackgroundSummary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := configuration(t)
	provider := blockedSummaryProvider{
		namedProvider: namedProvider{agenttest.NewScriptProvider(t, agenttest.TextTurn(strings.Repeat("long output ", 14000)))},
		started:       make(chan struct{}), finished: make(chan struct{}),
	}
	runtime := openContextRuntime(t, ctx, cfg, provider, core.Options{})
	defer closeRuntime(t, runtime)
	stream := runtime.Service.Start(ctx, execution.Request{Prompt: "inspect"})
	for update := range stream.Updates {
		if update.Result != nil && update.Result.ExitCode != 0 {
			t.Fatalf("interactive turn failed: %v", update.Error)
		}
	}
	select {
	case <-provider.started:
	case <-ctx.Done():
		t.Fatal("automatic summary never started")
	}
	select {
	case <-provider.finished:
		t.Fatal("summary ended with the originating turn")
	default:
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.finished:
	default:
		t.Fatal("runtime closed while summarizer was still running")
	}
}

func TestSessionNotesAreSharedWithChildrenAndIsolatedFromOtherSessions(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	provider := agenttest.NewScriptProvider(t,
		agenttest.ToolTurn("session_context_note", `{"note":"root handoff fact"}`),
		agenttest.TextTurn("noted"),
		agenttest.ToolTurn("session_context_note", `{"note":"child handoff fact"}`),
		agenttest.TextTurn("done"),
		agenttest.TextTurn("isolated"),
	)
	runtime := openContextRuntime(t, ctx, cfg, namedProvider{provider}, core.Options{})
	defer closeRuntime(t, runtime)
	parent, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "remember this"}, nil)
	if err != nil || parent.ExitCode != 0 {
		t.Fatal(parent, err)
	}
	child, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "use the handoff", ParentID: parent.ID}, nil)
	if err != nil || child.ExitCode != 0 || child.SessionID == parent.SessionID {
		t.Fatal(child, err)
	}
	memory, err := runtime.Service.SessionMemory.Read(ctx, parent.SessionID)
	if err != nil || !strings.Contains(memory.Markdown, "root handoff fact") || !strings.Contains(memory.Markdown, "child handoff fact") {
		t.Fatalf("notes did not share the main session: %+v %v", memory, err)
	}
	if _, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "different conversation"}, nil); err != nil {
		t.Fatal(err)
	}
	requests := provider.Requests()
	if !strings.Contains(requests[2].Context.Messages[0].Content[0].Text, "root handoff fact") ||
		!strings.Contains(requests[3].Context.Messages[0].Content[0].Text, "child handoff fact") ||
		requests[4].Context.Messages[0].Content[0].Text != "different conversation" {
		t.Fatal("inference received incorrect session scope")
	}
}

func TestCheckpointBeforeSessionContextToolStillResumesApproval(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	first := namedProvider{agenttest.NewScriptProvider(t, agenttest.ToolTurn("write_file", `{"path":"approved.txt","content":"done"}`))}
	runtime := openContextRuntime(t, ctx, cfg, first, core.Options{DisabledToolPacks: []string{"session-context"}})
	result, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "write the file"}, nil)
	if err != nil || result.Approval == nil {
		t.Fatal(result, err)
	}
	closeRuntime(t, runtime)

	resume := agenttest.NewScriptProvider(t, agenttest.TextTurn("approved"))
	runtime = openContextRuntime(t, ctx, cfg, namedProvider{resume}, core.Options{ResumeID: result.ID})
	defer closeRuntime(t, runtime)
	next, err := runtime.Service.Execute(ctx, execution.Request{ID: result.ID, Approve: result.Approval.ID}, nil)
	if err != nil || next.ExitCode != 0 {
		t.Fatal(next, err)
	}
	for _, request := range resume.Requests() {
		for _, tool := range request.Context.Tools {
			if tool.Name == execution.SessionContextNote {
				t.Fatal("new context tool leaked into the legacy approval generation")
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(cfg.Paths.Workspace, "approved.txt")); string(data) != "done" || err != nil {
		t.Fatalf("approved effect failed: %q %v", data, err)
	}
}

func openContextRuntime(t *testing.T, ctx context.Context, cfg config.Resolved, provider providers.Provider, options core.Options) *core.Runtime {
	t.Helper()
	options.ProviderFactory = func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }
	runtime, err := core.Open(ctx, cfg, options)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func TestOverBudgetToolExchangeIsCompactedAtRequestBoundary(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	cfg.Values["limits.context_tokens"] = config.Value{Value: 12000, Source: "test"}
	evidence := strings.Repeat("large unneeded output ", 2400)
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "evidence.txt"), []byte(evidence), 0600); err != nil {
		t.Fatal(err)
	}
	provider := agenttest.NewScriptProvider(t,
		agenttest.ToolTurn("read_file", `{"path":"evidence.txt"}`),
		agenttest.TextTurn(layeredSummary),
		agenttest.TextTurn(layeredSummary),
		agenttest.TextTurn("verified"),
	)
	runtime := openContextRuntime(t, ctx, cfg, namedProvider{provider}, core.Options{Profile: core.ProfileHeadless})
	defer closeRuntime(t, runtime)
	result, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "inspect this file"}, nil)
	if err != nil || result.ExitCode != 0 || result.Text != "verified" {
		t.Fatalf("active tool exchange could not continue: %+v %v", result, err)
	}
	last := provider.Requests()[3].Context.Messages
	if len(last) != 2 || !strings.Contains(last[0].Content[0].Text, layeredSummary) || last[1].Content[0].Text != "inspect this file" {
		t.Fatal("active objective or summary was missing at the next request")
	}
	ops, err := runtime.Service.Store.Operations(ctx, result.ID)
	if err != nil || len(ops) != 1 || ops[0].ToolName != "read_file" {
		t.Fatalf("tool was replayed or lost from operation journal: %v %v", ops, err)
	}
}

func TestInvalidManualSummaryDoesNotReplaceCheckpoint(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	runtime := open(t, cfg, agenttest.TextTurn("inspection complete"), agenttest.TextTurn("invalid summary"))
	defer closeRuntime(t, runtime)
	result, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "inspect"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Service.Compact(ctx, result.ID); err == nil {
		t.Fatal("manual compaction accepted a malformed summary")
	}
	after, err := runtime.Service.Store.Load(ctx, result.ID)
	if err != nil || after.Version != result.Checkpoint || len(after.Agent.Transcript) != 2 {
		t.Fatal("failed summary changed checkpoint", err)
	}
}

type delayedCompactionProvider struct{ namedProvider }

func (p delayedCompactionProvider) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	if strings.HasPrefix(req.Options.SessionID, "compaction-") {
		select {
		case <-time.After(11 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return p.namedProvider.Stream(ctx, req)
}

func TestHeadlessSummaryCanOutliveRunFinalizationDeadline(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runtime := openContextRuntime(t, ctx, configuration(t), delayedCompactionProvider{namedProvider{agenttest.NewScriptProvider(t,
		agenttest.TextTurn(strings.Repeat("verbose result to archive in the transcript. ", 3600)),
		agenttest.TextTurn(layeredSummary),
	)}}, core.Options{Profile: core.ProfileHeadless})
	defer closeRuntime(t, runtime)
	result, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "inspect"}, nil)
	if err != nil || result.State != "completed" || result.ExitCode != 0 {
		t.Fatalf("expected completed run despite summary delay: %s %v", result.State, err)
	}
	sessions, err := runtime.Service.Sessions(ctx, 0, 10)
	if err != nil || len(sessions) != 1 || sessions[0].State != "completed" {
		t.Fatalf("terminal state was not indexed: %v %v", sessions, err)
	}
	memory, err := runtime.Service.SessionMemory.Read(ctx, result.SessionID)
	if err != nil || !strings.Contains(memory.Markdown, layeredSummary) {
		t.Fatalf("headless run lost the summary: %v", err)
	}
}

func TestHistoricalRunCannotReplaceCurrentSessionContext(t *testing.T) {
	ctx := context.Background()
	runtime := openContextRuntime(t, ctx, configuration(t), namedProvider{agenttest.NewScriptProvider(t,
		agenttest.TextTurn("old goal completed"),
		agenttest.TextTurn(strings.Repeat("verbose result to archive in the transcript. ", 3600)),
		agenttest.TextTurn(layeredSummary),
	)}, core.Options{Profile: core.ProfileHeadless})
	defer closeRuntime(t, runtime)
	old, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "old goal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "new goal", ContinueFrom: old.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := runtime.Service.SessionMemory.Read(ctx, newer.SessionID)
	if err != nil || before.HeadRun != newer.ID || before.Projection.Count != 4 {
		t.Fatalf("missing current session context: %+v %v", before, err)
	}
	if err := runtime.Service.Compact(ctx, old.ID); err == nil || !strings.Contains(err.Error(), "outside the current session history") {
		t.Fatalf("historical compaction was not rejected: %v", err)
	}
	if _, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "diverge", ContinueFrom: old.ID}, nil); err == nil || !strings.Contains(err.Error(), "create a branch") {
		t.Fatalf("historical continuation was not directed to a branch: %v", err)
	}
	after, err := runtime.Service.SessionMemory.Read(ctx, newer.SessionID)
	if err != nil || before != after {
		t.Fatalf("historical actions changed the latest memory: before %+v after %+v, %v", before, after, err)
	}
}

type unavailableContextMirror struct{}

func (unavailableContextMirror) WriteContext(context.Context, string, string) error {
	return os.ErrPermission
}

type checkpointWriteFault struct {
	execution.ExecutionStore
	fail bool
}

func (f *checkpointWriteFault) Save(ctx context.Context, id string, version uint64, state agent.Snapshot) error {
	if f.fail {
		f.fail = false
		return errors.New("injected snapshot write failure")
	}
	return f.ExecutionStore.Save(ctx, id, version, state)
}

type archiveWriteFault struct {
	summarizer.Store
	failVersion uint64
}

func (f *archiveWriteFault) CAS(ctx context.Context, bucket, key string, version uint64, value any) error {
	if bucket == "compactions" && version == f.failVersion {
		f.failVersion = 0
		return errors.New("injected archive update failure")
	}
	return f.Store.CAS(ctx, bucket, key, version, value)
}

func TestManualCompactionRecoversAcrossDurableWrites(t *testing.T) {
	for _, phase := range []string{"published", "checkpoint", "archive", "mirror"} {
		t.Run(phase, func(t *testing.T) {
			cfg := configuration(t)
			ctx := context.Background()
			firstProvider := agenttest.NewScriptProvider(t, agenttest.TextTurn("inspection complete"), agenttest.TextTurn(layeredSummary))
			runtime := openContextRuntime(t, ctx, cfg, namedProvider{firstProvider}, core.Options{Profile: core.ProfileHeadless})
			result, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "inspect"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			switch phase {
			case "checkpoint":
				runtime.Service.Store = &checkpointWriteFault{ExecutionStore: runtime.Service.Store, fail: true}
			case "published":
				runtime.Service.SessionMemory.Store = &archiveWriteFault{Store: runtime.Service.SessionMemory.Store, failVersion: 1}
			case "archive":
				runtime.Service.SessionMemory.Store = &archiveWriteFault{Store: runtime.Service.SessionMemory.Store, failVersion: 2}
			case "mirror":
				runtime.Service.SessionMemory.File = unavailableContextMirror{}
			}
			err = runtime.Service.Compact(ctx, result.ID)
			if (err != nil) != (phase != "mirror") {
				t.Fatalf("unexpected result at %s: %v", phase, err)
			}
			closeRuntime(t, runtime)

			nextProvider := agenttest.NewScriptProvider(t, agenttest.TextTurn("continuation"))
			runtime = openContextRuntime(t, ctx, cfg, namedProvider{nextProvider}, core.Options{Profile: core.ProfileHeadless})
			defer closeRuntime(t, runtime)
			// The canonical projection already covers the original checkpoint.
			// The next run can advance while the archived rewrite is retried.
			if phase == "checkpoint" {
				if _, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "continue", ContinueFrom: result.ID}, nil); err != nil {
					t.Fatal(err)
				}
			}
			if err := runtime.Service.Compact(ctx, result.ID); err != nil {
				t.Fatal("retry should resume the same compaction", err)
			}
			snapshot, err := runtime.Service.Store.Load(ctx, result.ID)
			if err != nil || snapshot.Version != result.Checkpoint+1 || len(snapshot.Agent.Transcript) != 1 {
				t.Fatalf("expected checkpoint %d and one message, found version %d and %d messages: %v", result.Checkpoint+1, snapshot.Version, len(snapshot.Agent.Transcript), err)
			}
			archives, err := runtime.Service.Records.List(ctx, "compactions")
			if err != nil || len(archives) != 1 {
				t.Fatal("expected a single durable compaction archive", err)
			}
			var archive struct{ State string }
			if err := json.Unmarshal(archives[0].Data, &archive); err != nil || archive.State != "committed" {
				t.Fatalf("archive was not finalized: %+v %v", archive, err)
			}
			if phase != "checkpoint" {
				if _, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "continue", ContinueFrom: result.ID}, nil); err != nil {
					t.Fatal(err)
				}
			}
			requests := nextProvider.Requests()
			if len(requests) != 1 || len(requests[0].Context.Messages) != 2 ||
				!strings.Contains(requests[0].Context.Messages[0].Content[0].Text, layeredSummary) {
				t.Fatal("retry re-ran inference or duplicated the compacted context")
			}
		})
	}
}

type contextThresholdProvider struct {
	namedProvider
	threshold int64
}

func (p contextThresholdProvider) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	if len(p.Requests()) == 0 {
		delta := p.threshold - providers.EstimateTokens(req.Context) + 4
		p.AppendTurns(agenttest.TextTurn(strings.Repeat("abcd", int(delta))), agenttest.TextTurn(layeredSummary))
	}
	return p.namedProvider.Stream(ctx, req)
}

func TestCompletedWindowIncludesAdvertisedToolSchemas(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	cfg.Values["limits.context_tokens"] = config.Value{Value: 16000, Source: "test"}
	provider := contextThresholdProvider{namedProvider{agenttest.NewScriptProvider(t)}, 9600}
	pack := core.ToolPack{ID: "large-schema", Register: func(_ context.Context, r *tools.Registry, _ core.ToolPackContext) error {
		return r.Add(tools.ToolDescriptor{Name: "measure_large_schema", Description: strings.Repeat("schema content ", 400)},
			tools.ToolHandlerFunc(func(context.Context, tools.ToolRequest) (tools.ToolResponse, error) {
				return tools.ToolResponse{}, nil
			}))
	}}
	runtime := openContextRuntime(t, ctx, cfg, provider, core.Options{Profile: core.ProfileHeadless, ToolPacks: []core.ToolPack{pack}})
	defer closeRuntime(t, runtime)
	result, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "inspect"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	memory, err := runtime.Service.SessionMemory.Read(ctx, result.SessionID)
	if err != nil || memory.Projection.Count != 2 {
		t.Fatalf("completed forecast missed compaction at the measured threshold: %+v %v", memory, err)
	}
}

func TestChildCompactionKeepsItsProjectionPrivate(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	cfg.Values["limits.context_tokens"] = config.Value{Value: 12000, Source: "test"}
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "large.txt"), []byte(strings.Repeat("large unneeded output ", 2400)), 0600); err != nil {
		t.Fatal(err)
	}
	provider := agenttest.NewScriptProvider(t,
		agenttest.TextTurn("root complete"),
		agenttest.ToolTurn("read_file", `{"path":"large.txt"}`),
		agenttest.TextTurn(layeredSummary),
		agenttest.TextTurn(layeredSummary),
		agenttest.TextTurn("child verified"),
	)
	runtime := openContextRuntime(t, ctx, cfg, namedProvider{provider}, core.Options{Profile: core.ProfileHeadless})
	defer closeRuntime(t, runtime)
	parent, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "root task"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Service.SessionMemory.Note(ctx, parent.SessionID, "root handoff fact"); err != nil {
		t.Fatal(err)
	}
	before, err := runtime.Service.SessionMemory.Read(ctx, parent.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	child, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "inspect child file", ParentID: parent.ID}, nil)
	if err != nil || child.Text != "child verified" {
		t.Fatalf("child failed to compact an over-budget request: %+v %v", child, err)
	}
	after, err := runtime.Service.SessionMemory.Read(ctx, parent.SessionID)
	if err != nil || before != after {
		t.Fatal("child projection changed canonical root memory", err)
	}
	private, err := runtime.Service.SessionMemory.ReadPrivate(ctx, child.ID)
	if err != nil || private.Projection.Count == 0 {
		t.Fatal("child has no private projection", err)
	}
	requests := provider.Requests()
	last := requests[len(requests)-1].Context.Messages
	if len(last) != 3 || !strings.Contains(last[0].Content[0].Text, "root handoff fact") ||
		!strings.Contains(last[1].Content[0].Text, layeredSummary) || last[2].Content[0].Text != "inspect child file" {
		t.Fatal("child lost its task, shared notes or compacted evidence")
	}
}

type blockedContinuationProvider struct {
	namedProvider
	started chan struct{}
	release chan struct{}
}

func (p blockedContinuationProvider) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	close(p.started)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.release:
		return p.namedProvider.Stream(ctx, req)
	}
}

func TestConcurrentContinuationsKeepTheWinningSessionHead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := configuration(t)
	original := open(t, cfg, agenttest.TextTurn("root complete"))
	first, err := original.Service.Execute(ctx, execution.Request{Prompt: "root task"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, original)

	blocked := blockedContinuationProvider{
		namedProvider: namedProvider{agenttest.NewScriptProvider(t, agenttest.TextTurn("parallel complete"))},
		started:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	parallel := openContextRuntime(t, ctx, cfg, blocked, core.Options{Profile: core.ProfileHeadless})
	defer closeRuntime(t, parallel)
	stream := parallel.Service.Start(ctx, execution.Request{Prompt: "parallel task", ContinueFrom: first.ID})
	select {
	case <-blocked.started:
	case <-ctx.Done():
		t.Fatal("parallel run never entered inference")
	}
	current := open(t, cfg, agenttest.TextTurn("current complete"))
	defer closeRuntime(t, current)
	winner, err := current.Service.Execute(ctx, execution.Request{Prompt: "current task", ContinueFrom: first.ID}, nil)
	if err != nil || winner.State != "completed" {
		t.Fatalf("current run did not complete: %s %v", winner.State, err)
	}
	close(blocked.release)
	var loser execution.Result
	for update := range stream.Updates {
		if update.Result != nil {
			loser = *update.Result
			if update.Error != nil {
				t.Fatal(update.Error)
			}
		}
	}
	if loser.State != "completed" {
		t.Fatalf("parallel run did not complete: %s", loser.State)
	}
	memory, err := current.Service.SessionMemory.Read(ctx, first.SessionID)
	if err != nil || memory.HeadRun != winner.ID {
		t.Fatalf("parallel completion replaced the session head: %q %v", memory.HeadRun, err)
	}
	inspection, err := core.Open(ctx, cfg, core.Options{Profile: core.ProfileInspection})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, inspection)
	sessions, err := inspection.Service.Sessions(ctx, 0, 10)
	if err != nil || len(sessions) != 1 || sessions[0].ID != winner.ID {
		t.Fatalf("session selection exposed the losing continuation: %+v %v", sessions, err)
	}
}

func TestLegacySessionHeadUsesCheckpointCompletionOnReplay(t *testing.T) {
	ctx := context.Background()
	runtime := open(t, configuration(t), agenttest.TextTurn("old answer"), agenttest.TextTurn("new answer"))
	defer closeRuntime(t, runtime)
	old, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "old"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	newest, err := runtime.Service.Execute(ctx, execution.Request{Prompt: "new", ContinueFrom: old.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	memory, err := runtime.Service.SessionMemory.Read(ctx, newest.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	memory.HeadRun = ""
	if err := runtime.Service.SessionMemory.Commit(ctx, newest.SessionID, memory); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Service.Execute(ctx, execution.Request{ID: old.ID}, nil); err != nil {
		t.Fatal(err)
	}
	memory, err = runtime.Service.SessionMemory.Read(ctx, newest.SessionID)
	if err != nil || memory.HeadRun != newest.ID {
		t.Fatalf("historical replay took over a legacy session: %q %v", memory.HeadRun, err)
	}
}
