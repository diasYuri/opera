package harness_test

import (
	"context"
	"encoding/json"
	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/harness"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/providertest"
	yruntime "github.com/diasYuri/y/pkg/runtime"
	"github.com/diasYuri/y/pkg/tools"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type workerProvider struct{ *providertest.FakeProvider }

func (p workerProvider) ID() string { return "openai" }
func (p workerProvider) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	text := ""
	done := map[string]bool{}
	for _, message := range req.Context.Messages {
		if message.Role == ai.RoleUser && text == "" {
			for _, block := range message.Content {
				text += block.Text
			}
		}
		if message.ToolResult != nil {
			done[message.ToolResult.ToolName] = true
		}
	}
	events := []ai.Event{}
	call := func(id, name, args string) {
		events = append(events, ai.ToolCallEvent{ContentIndex: len(events), ToolCall: ai.ToolCall{ID: id, Name: name, Arguments: json.RawMessage(args)}, Complete: true})
	}
	if strings.HasPrefix(text, "child-") {
		if !done["read_file"] {
			call("read", "read_file", `{"path":"input.txt"}`)
		} else if !done["write_file"] {
			raw, _ := json.Marshal(map[string]string{"path": text + ".txt", "content": "verified"})
			call("write", "write_file", string(raw))
		} else {
			events = append(events, ai.TextDelta{Text: "child verified"})
		}
	} else {
		if !done["subagent_spawn"] {
			call("one", "subagent_spawn", `{"name":"one","prompt":"child-one"}`)
			call("two", "subagent_spawn", `{"name":"two","prompt":"child-two"}`)
		} else if !done["task_complete"] {
			call("complete", "task_complete", `{"id":"task-1"}`)
		} else {
			events = append(events, ai.TextDelta{Text: "worker verified"})
		}
	}
	events = append(events, ai.StopEvent{Reason: ai.StopReasonStop})
	fake := providertest.NewFakeProvider(providertest.WithFakeResponses(providertest.FakeResponse{Events: events}))
	return fake.Stream(ctx, req)
}
func workerRuntime(t *testing.T, cfg config.Resolved) *harness.Runtime {
	t.Helper()
	r, err := harness.Open(context.Background(), cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		return workerProvider{providertest.NewFakeProvider()}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestWorkerChildrenApprovalRecoveryAndTaskIdentity(t *testing.T) {
	cfg := configuration(t)
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "input.txt"), []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r := workerRuntime(t, cfg)
	result, err := r.Service.Execute(ctx, app.Request{Prompt: "parent", Worker: true}, nil)
	if err != nil || result.ExitCode != 3 || result.Approval == nil {
		t.Fatalf("%+v %v", result, err)
	}
	oldApproval := result.Approval.ID
	if err := os.WriteFile(result.Approval.Path, []byte("external change"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := r.Service.Execute(ctx, app.Request{ID: result.ID, Approve: oldApproval}, nil)
	if err != nil || changed.Approval == nil || changed.Approval.ID == oldApproval {
		t.Fatalf("stale child approval was reused: %+v %v", changed, err)
	}
	result = changed
	id := result.ID
	children, err := r.Service.Work.Children(ctx, id)
	if err != nil || len(children) != 2 {
		t.Fatalf("%v %v", children, err)
	}
	ids := []string{children[0].ID, children[1].ID}
	closeRuntime(t, r)
	r = workerRuntime(t, cfg)
	defer closeRuntime(t, r)
	for i := 0; i < 3 && result.ExitCode == 3; i++ {
		result, err = r.Service.Execute(ctx, app.Request{ID: id, Approve: result.Approval.ID}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	children, err = r.Service.Work.Children(ctx, id)
	if err != nil || children[0].ID != ids[0] || children[1].ID != ids[1] {
		t.Fatal("child identities changed", err)
	}
	tasks, err := r.Service.Work.Tasks(ctx, id)
	if err != nil || len(tasks.Tasks) != 1 || tasks.Tasks[0].ID != "task-1" {
		t.Fatal("task identity lost", tasks, err)
	}
	budgets, err := r.Service.Records.List(ctx, "budgets")
	if err != nil || len(budgets) != 1 {
		t.Fatal("budget not shared", len(budgets), err)
	}
	for _, name := range []string{"child-one.txt", "child-two.txt"} {
		data, err := os.ReadFile(filepath.Join(cfg.Paths.Workspace, name))
		if err != nil || string(data) != "verified" {
			t.Fatal(name, err)
		}
	}
}
func TestWorkerParallelReadsAndSerializedWrites(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["policy"] = config.Value{Value: "trusted", Source: "test"}
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "input.txt"), []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	r := workerRuntime(t, cfg)
	defer closeRuntime(t, r)
	registry, err := yruntime.Resolve(r.Harness, yruntime.ToolRegistryKey)
	if err != nil {
		t.Fatal(err)
	}
	var readers, writers, maxWriters atomic.Int32
	both := make(chan struct{})
	var once sync.Once
	err = registry.AddExecutionMiddleware(func(ctx context.Context, req tools.ToolRequest, desc tools.ToolDescriptor, next tools.ToolExecutionNext) (tools.ToolResponse, error) {
		if req.Name == "read_file" {
			if readers.Add(1) == 2 {
				once.Do(func() { close(both) })
			}
			select {
			case <-both:
			case <-ctx.Done():
				return tools.ToolResponse{}, ctx.Err()
			}
			defer readers.Add(-1)
		}
		if req.Name == "write_file" {
			n := writers.Add(1)
			for old := maxWriters.Load(); n > old && !maxWriters.CompareAndSwap(old, n); old = maxWriters.Load() {
			}
			defer writers.Add(-1)
			time.Sleep(5 * time.Millisecond)
		}
		return next(ctx, req)
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := r.Service.Execute(ctx, app.Request{Prompt: "parent", Worker: true}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	if maxWriters.Load() != 1 {
		t.Fatal("mutations overlapped", maxWriters.Load())
	}
	select {
	case <-both:
	default:
		t.Fatal("reads serialized")
	}
}

type blockingChildren struct {
	workerProvider
	entered chan struct{}
}

func (p blockingChildren) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	for _, m := range req.Context.Messages {
		if m.Role == ai.RoleUser {
			for _, b := range m.Content {
				if strings.HasPrefix(b.Text, "child-") {
					p.entered <- struct{}{}
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}
			break
		}
	}
	return p.workerProvider.Stream(ctx, req)
}
func TestWorkerCancellationPropagatesAndInspectionDoesNotRestart(t *testing.T) {
	cfg := configuration(t)
	entered := make(chan struct{}, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := harness.Open(ctx, cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		return blockingChildren{workerProvider{providertest.NewFakeProvider()}, entered}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	execution := r.Service.Start(ctx, app.Request{Prompt: "parent", Worker: true})
	results := make(chan app.Result, 1)
	go func() {
		var terminal app.Result
		for update := range execution.Updates {
			if update.Result != nil {
				terminal = *update.Result
			}
		}
		results <- terminal
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("children did not start")
		}
	}
	execution.Cancel()
	result := <-results
	if result.ExitCode != 130 && result.ExitCode != 4 {
		t.Fatalf("%+v", result)
	}
	if result.ExitCode == 4 {
		operations, err := r.Service.Store.Operations(ctx, execution.ID)
		if err != nil {
			t.Fatal(err)
		}
		pending := false
		for _, op := range operations {
			if op.State == agent.OperationPending {
				pending = true
			}
		}
		if !pending {
			t.Fatal("uncertain cancellation had no pending operation")
		}
	}
	closeRuntime(t, r)
	inspection, err := harness.Open(ctx, cfg, harness.Options{Inspection: true})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, inspection)
	view, err := inspection.Service.Inspect(ctx, execution.ID)
	if err != nil || len(view.Children) != 2 {
		t.Fatal(err)
	}
	if inspection.Service.Active() {
		t.Fatal("inspection restarted execution")
	}
}

func TestActiveControlsAreOrderedAndPersisted(t *testing.T) {
	cfg := configuration(t)
	provider := &cancellableProvider{FakeProvider: providertest.NewFakeProvider(), entered: make(chan struct{}), block: true}
	r, err := harness.Open(context.Background(), cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, r)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	execution := r.Service.Start(ctx, app.Request{Prompt: "begin"})
	select {
	case <-provider.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for _, text := range []string{"first follow-up", "second follow-up"} {
		if err := r.FollowUp(execution.ID, text); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Steer(execution.ID, "change direction"); err != nil {
		t.Fatal(err)
	}
	execution.Cancel()
	for range execution.Updates {
	}
	snapshot, err := r.Service.Store.Load(ctx, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Agent.FollowUpQueue) != 2 || snapshot.Agent.FollowUpQueue[0].Content[0].Text != "first follow-up" || snapshot.Agent.FollowUpQueue[1].Content[0].Text != "second follow-up" {
		t.Fatal("follow-up order was not persisted", snapshot.Agent.FollowUpQueue)
	}
	if len(snapshot.Agent.PendingSteering) != 1 {
		t.Fatal("steering was not checkpointed")
	}
	controls, err := r.Service.Records.List(ctx, "controls")
	if err != nil || len(controls) != 3 {
		t.Fatal("control journal missing", err)
	}
}

type nestedProvider struct{ *providertest.FakeProvider }

func (p nestedProvider) ID() string { return "openai" }
func (p nestedProvider) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	text := ""
	spawned := false
	for _, m := range req.Context.Messages {
		if m.Role == ai.RoleUser && text == "" {
			for _, b := range m.Content {
				text += b.Text
			}
		}
		if m.ToolResult != nil && m.ToolResult.ToolName == "subagent_spawn" {
			spawned = true
		}
	}
	events := []ai.Event{}
	if text != "node-3" && !spawned {
		next := "node-2"
		if text == "node-2" {
			next = "node-3"
		}
		raw, _ := json.Marshal(map[string]string{"prompt": next})
		events = append(events, ai.ToolCallEvent{ToolCall: ai.ToolCall{ID: "spawn", Name: "subagent_spawn", Arguments: raw}, Complete: true})
	} else {
		events = append(events, ai.TextDelta{Text: "nested result"})
	}
	events = append(events, ai.StopEvent{Reason: ai.StopReasonStop})
	return providertest.NewFakeProvider(providertest.WithFakeResponses(providertest.FakeResponse{Events: events})).Stream(ctx, req)
}
func TestNestedChildrenYieldSingleWorkerSlot(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["agents.max_concurrent"] = config.Value{Value: 1, Source: "test"}
	cfg.Values["agents.max_depth"] = config.Value{Value: 2, Source: "test"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := harness.Open(ctx, cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		return nestedProvider{providertest.NewFakeProvider()}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, r)
	result, err := r.Service.Execute(ctx, app.Request{Prompt: "node-1"}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	children, err := r.Service.Work.Children(ctx, result.ID)
	if err != nil || len(children) != 1 {
		t.Fatal(children, err)
	}
	grandchildren, err := r.Service.Work.Children(ctx, children[0].ID)
	if err != nil || len(grandchildren) != 1 || grandchildren[0].Depth != 2 {
		t.Fatal("nested hierarchy missing", grandchildren, err)
	}
}
func TestSharedBudgetFailsBeforeInference(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["limits.worker_tokens"] = config.Value{Value: 1024, Source: "test"}
	provider := providertest.NewFakeProvider()
	r, err := harness.Open(context.Background(), cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, r)
	result, err := r.Service.Execute(context.Background(), app.Request{Prompt: "budget", Worker: true}, nil)
	if err == nil || result.ExitCode == 0 || provider.CallCount() != 0 {
		t.Fatal("inference bypassed shared budget", result, err)
	}
}

type continuingProvider struct{ *providertest.FakeProvider }

func (p continuingProvider) ID() string { return "openai" }
func (p continuingProvider) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	return providertest.NewFakeProvider(providertest.WithFakeResponses(providertest.FakeResponse{Events: []ai.Event{ai.TextDelta{Text: "continued"}, ai.StopEvent{Reason: ai.StopReasonStop}}})).Stream(ctx, req)
}
func TestControlJournalRecoversMissingQueueExactlyOnce(t *testing.T) {
	cfg := configuration(t)
	provider := &cancellableProvider{FakeProvider: providertest.NewFakeProvider(), entered: make(chan struct{}), block: true}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := harness.Open(ctx, cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	execution := r.Service.Start(ctx, app.Request{Prompt: "begin"})
	<-provider.entered
	if err := r.FollowUp(execution.ID, "durable follow-up"); err != nil {
		t.Fatal(err)
	}
	execution.Cancel()
	for range execution.Updates {
	}
	state, err := r.Service.Store.Load(ctx, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.Agent.FollowUpQueue = nil
	state.Agent.PendingSteering = nil
	if err := r.Service.Store.Save(ctx, execution.ID, state.Version, state); err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, r)
	r, err = harness.Open(ctx, cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		return continuingProvider{providertest.NewFakeProvider()}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, r)
	result, err := r.Service.Execute(ctx, app.Request{ID: execution.ID}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	snapshot, err := r.Service.Store.Load(ctx, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, message := range snapshot.Agent.Transcript {
		for _, block := range message.Content {
			if block.Text == "durable follow-up" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatal("control redelivered or lost", count)
	}
	if _, err := r.Service.Execute(ctx, app.Request{ID: execution.ID}, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err = r.Service.Store.Load(ctx, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	for _, message := range snapshot.Agent.Transcript {
		for _, block := range message.Content {
			if block.Text == "durable follow-up" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatal("second resume duplicated a control", count)
	}
}

func TestWorkerDenialDoesNotAuthorizeAnotherChild(t *testing.T) {
	cfg := configuration(t)
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "input.txt"), []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	r := workerRuntime(t, cfg)
	defer closeRuntime(t, r)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := r.Service.Execute(ctx, app.Request{Prompt: "parent", Worker: true}, nil)
	if err != nil || result.Approval == nil {
		t.Fatalf("%+v %v", result, err)
	}
	deniedPath := result.Approval.Path
	deniedID := result.Approval.ID
	id := result.ID
	result, err = r.Service.Execute(ctx, app.Request{ID: id, Deny: deniedID}, nil)
	if err != nil || result.ExitCode != 3 || result.Approval == nil || result.Approval.ID == deniedID {
		t.Fatalf("denial authorized another child: %+v %v", result, err)
	}
	approvedPath := result.Approval.Path
	result, err = r.Service.Execute(ctx, app.Request{ID: id, Approve: result.Approval.ID}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	if _, err := os.Stat(deniedPath); !os.IsNotExist(err) {
		t.Fatal("denied child wrote a file", err)
	}
	if data, err := os.ReadFile(approvedPath); err != nil || string(data) != "verified" {
		t.Fatal("approved child did not write", err)
	}
}
