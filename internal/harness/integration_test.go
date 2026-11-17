package harness_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/harness"
	"github.com/diasYuri/opera/internal/platform"
	"github.com/diasYuri/opera/internal/providercatalog"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/providertest"
	"github.com/diasYuri/y/pkg/tools"
)

type namedProvider struct{ *agenttest.ScriptProvider }

func (namedProvider) ID() string { return "openai" }

func configuration(t *testing.T) config.Resolved {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	paths, err := platform.Resolve(workspace, root, "linux", func(key string) string {
		return map[string]string{"OPERA_CONFIG_DIR": filepath.Join(root, "config"), "OPERA_DATA_DIR": filepath.Join(root, "data"), "OPERA_CACHE_DIR": filepath.Join(root, "cache")}[key]
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{Paths: paths, Flags: map[string]string{"provider": "openai", "model": "fixture", "policy": "ask"}})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

type cancellableProvider struct {
	*providertest.FakeProvider
	entered chan struct{}
	block   bool
}

func (p *cancellableProvider) ID() string { return "openai" }
func (p *cancellableProvider) Stream(ctx context.Context, request providers.StreamRequest) (providers.EventStream, error) {
	if p.block {
		close(p.entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return p.FakeProvider.Stream(ctx, request)
}

func TestCancellationPersistsAndExplicitResumeContinues(t *testing.T) {
	cfg := configuration(t)
	ctx := context.Background()
	provider := &cancellableProvider{FakeProvider: providertest.NewFakeProvider(), entered: make(chan struct{}), block: true}
	runtime, err := harness.Open(ctx, cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	execution := runtime.Service.Start(ctx, app.Request{Prompt: "start"})
	<-provider.entered
	execution.Cancel()
	var cancelled app.Result
	for update := range execution.Updates {
		if update.Result != nil {
			cancelled = *update.Result
		}
	}
	if cancelled.ExitCode != 130 || cancelled.Checkpoint == 0 {
		t.Fatalf("cancellation lost checkpoint/code: %#v", cancelled)
	}
	closeRuntime(t, runtime)
	resumed := open(t, cfg, agenttest.TextTurn("recovered"))
	defer closeRuntime(t, resumed)
	result, err := resumed.Service.Execute(ctx, app.Request{ID: cancelled.ID}, nil)
	if err != nil || result.ExitCode != 0 || result.Text != "recovered" {
		t.Fatalf("explicit recovery failed: %#v %v", result, err)
	}
}

func TestCloseCancelsAndDrainsApplicationBeforeStores(t *testing.T) {
	cfg := configuration(t)
	provider := &cancellableProvider{FakeProvider: providertest.NewFakeProvider(), entered: make(chan struct{}), block: true}
	runtime, err := harness.Open(context.Background(), cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	execution := runtime.Service.Start(context.Background(), app.Request{Prompt: "start"})
	<-provider.entered
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.Close(ctx); err != nil {
		t.Fatal("application did not drain before storage shutdown", err)
	}
	var result app.Result
	for update := range execution.Updates {
		if update.Result != nil {
			result = *update.Result
		}
	}
	if result.ExitCode != 130 || result.Checkpoint == 0 {
		t.Fatal("shutdown lost the final checkpoint")
	}
	reopened := open(t, cfg, agenttest.TextTurn("resumed"))
	defer closeRuntime(t, reopened)
	resumed, err := reopened.Service.Execute(context.Background(), app.Request{ID: result.ID}, nil)
	if err != nil || resumed.ExitCode != 0 {
		t.Fatal("shutdown checkpoint was not resumable", err)
	}
}

func TestUncertainEffectBlocksUntilExplicitReconciliation(t *testing.T) {
	cfg := configuration(t)
	ctx := context.Background()
	runtime := open(t, cfg, agenttest.TextTurn("done"))
	defer closeRuntime(t, runtime)
	first, err := runtime.Service.Execute(ctx, app.Request{Prompt: "hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, acquired, err := runtime.Service.Store.BeginOperation(ctx, agent.OperationRecord{RunID: first.ID, IdempotencyKey: "uncertain", ToolName: "write_file", ArgumentsHash: "hash"}); err != nil || !acquired {
		t.Fatal(err)
	}
	blocked, err := runtime.Service.Execute(ctx, app.Request{ID: first.ID}, nil)
	if err == nil || blocked.ExitCode != 4 {
		t.Fatal("uncertain operation did not block resume")
	}
	if err := runtime.Service.Store.CompleteOperation(ctx, "uncertain", tools.ToolResponse{Content: []tools.ContentBlock{{Type: tools.ContentText, Text: "user confirmed observed effect"}}}, nil); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Service.Execute(ctx, app.Request{ID: first.ID}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatal("explicit reconciliation did not unblock cached result", err)
	}
}
func open(t *testing.T, cfg config.Resolved, turns ...agenttest.ScriptedTurn) *harness.Runtime {
	t.Helper()
	provider := namedProvider{agenttest.NewScriptProvider(t, turns...)}
	runtime, err := harness.Open(context.Background(), cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }, Credential: func(context.Context, string) (string, error) { return "fixture-secret-never-persist", nil }})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}
func closeRuntime(t *testing.T, runtime *harness.Runtime) {
	t.Helper()
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestVerticalApprovalCheckpointAndResume(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	one := open(t, cfg, agenttest.TextAndToolTurn("Preparing change", "write_file", `{"path":"result.txt","content":"verified"}`))
	var text strings.Builder
	first, err := one.Service.Execute(ctx, app.Request{Prompt: "write the result"}, func(event app.Event) error { text.WriteString(event.Text); return nil })
	if err != nil || first.ExitCode != 3 || first.Approval == nil || !strings.Contains(text.String(), "Preparing change") {
		t.Fatalf("first execution: %#v %v", first, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.Workspace, "result.txt")); !os.IsNotExist(err) {
		t.Fatal("mutation ran without approval")
	}
	state, err := one.Service.Store.Load(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(state)
	if err != nil || strings.Contains(string(raw), "fixture-secret-never-persist") {
		t.Fatal("credential leaked to checkpoint", err)
	}
	closeRuntime(t, one)
	two := open(t, cfg, agenttest.TextTurn("Change verified"))
	second, err := two.Service.Execute(ctx, app.Request{ID: first.ID, Approve: first.Approval.ID}, nil)
	if err != nil || second.ExitCode != 0 {
		t.Fatalf("resume: %#v %v", second, err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.Paths.Workspace, "result.txt"))
	if err != nil || string(data) != "verified" {
		t.Fatal("approved write missing", err)
	}
	ops, err := two.Service.Store.Operations(ctx, first.ID)
	if err != nil || len(ops) != 1 {
		t.Fatal("durable operations missing", err)
	}
	replayed, err := two.Service.Execute(ctx, app.Request{ID: first.ID}, nil)
	if err != nil || replayed.ExitCode != 0 {
		t.Fatal("completed replay failed", err)
	}
	closeRuntime(t, two)
}

func TestDenialAndChangedDiffRequireReview(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	path := filepath.Join(cfg.Paths.Workspace, "result.txt")
	one := open(t, cfg, agenttest.ToolTurn("write_file", `{"path":"result.txt","content":"candidate"}`), agenttest.TextTurn("Denied"))
	defer closeRuntime(t, one)
	first, err := one.Service.Execute(ctx, app.Request{Prompt: "change"}, nil)
	if err != nil || first.Approval == nil {
		t.Fatal("missing approval", err)
	}
	if err := os.WriteFile(path, []byte("concurrent change"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := one.Service.Execute(ctx, app.Request{ID: first.ID, Approve: first.Approval.ID}, nil)
	if err != nil || changed.ExitCode != 3 || changed.Approval.ID == first.Approval.ID {
		t.Fatal("stale diff was approved", err)
	}
	denied, err := one.Service.Execute(ctx, app.Request{ID: first.ID, Deny: changed.Approval.ID}, nil)
	if err != nil || denied.ExitCode != 0 {
		t.Fatal("denial failed", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "concurrent change" {
		t.Fatal("denial mutated the file", err)
	}
}

func TestStartupFailureReleasesScope(t *testing.T) {
	cfg := configuration(t)
	_, err := harness.Open(context.Background(), cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		return nil, errors.New("fixture startup failure")
	}})
	if err == nil {
		t.Fatal("startup failure swallowed")
	}
	runtime := open(t, cfg, agenttest.TextTurn("done"))
	defer closeRuntime(t, runtime)
	if _, err := runtime.Service.Execute(context.Background(), app.Request{Prompt: "hello"}, nil); err != nil {
		t.Fatal(err)
	}
}

type observedCloseProvider struct {
	*providertest.FakeProvider
	closed *int
	order  *[]string
}

func (p *observedCloseProvider) ID() string { return "openai" }
func (p *observedCloseProvider) Close() error {
	*p.closed++
	*p.order = append(*p.order, "provider")
	return p.FakeProvider.Close()
}

func TestStartupFailureRollsBackProviderAndToolPackResources(t *testing.T) {
	cfg := configuration(t)
	var closes []string
	providerClosed := 0
	provider := &observedCloseProvider{FakeProvider: providertest.NewFakeProvider(), closed: &providerClosed, order: &closes}
	failure := errors.New("tool pack startup failed")
	_, err := harness.Open(context.Background(), cfg, harness.Options{
		ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil },
		ToolPacks: []harness.ToolPack{{ID: "failing-resource", Register: func(_ context.Context, registry *tools.Registry, _ harness.ToolPackContext) error {
			if err := registry.AddCloser(func(context.Context) error { closes = append(closes, "tool-resource"); return nil }); err != nil {
				return err
			}
			return failure
		}}},
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Open error = %v, want tool pack failure", err)
	}
	if providerClosed != 1 {
		t.Fatalf("provider close count = %d, want 1", providerClosed)
	}
	if len(closes) != 2 || closes[0] != "tool-resource" || closes[1] != "provider" {
		t.Fatalf("resource cleanup = %v", closes)
	}
}

func TestInspectionDoesNotCreateProvider(t *testing.T) {
	cfg := configuration(t)
	created := false
	runtime, err := harness.Open(context.Background(), cfg, harness.Options{Inspection: true, ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		created = true
		return providertest.NewFakeProvider(), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)
	if created {
		t.Fatal("inspection created a provider")
	}
}

type catalogScriptProvider struct{ *agenttest.ScriptProvider }

func (catalogScriptProvider) ID() string { return "example" }

func TestCustomProviderCatalogExecutesOwnImplementation(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["provider"] = config.Value{Value: "example", Source: "test"}
	cfg.Values["model"] = config.Value{Value: "scripted", Source: "test"}
	provider := catalogScriptProvider{agenttest.NewScriptProvider(t, agenttest.TextTurn("answer from the custom provider"))}
	catalog, err := providercatalog.New(providercatalog.Entry{ID: "example", Name: "Example", Factory: func(context.Context, config.Resolved, func(context.Context, string) (string, error)) (providers.Provider, error) {
		return provider, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := harness.Open(context.Background(), cfg, harness.Options{ProviderCatalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)
	result, err := runtime.Service.Execute(context.Background(), app.Request{Prompt: "say hello"}, nil)
	if err != nil || result.Text != "answer from the custom provider" {
		t.Fatalf("custom provider result = %#v, %v", result, err)
	}
}

func TestCancelledStartupDoesNotCreateProvider(t *testing.T) {
	cfg := configuration(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	created := false
	_, err := harness.Open(ctx, cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		created = true
		return providertest.NewFakeProvider(), nil
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("startup error = %v, want cancellation", err)
	}
	if created {
		t.Fatal("cancelled startup created a provider")
	}
}

func TestContextPathSourceTracksRuntimeUpdates(t *testing.T) {
	cfg := configuration(t)
	runtime, err := harness.Open(context.Background(), cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		return providertest.NewFakeProvider(), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)
	runtime.SetContextPaths([]string{"docs/guide.md"})
	got := runtime.Service.ContextPathSource.Current()
	if len(got) != 1 || got[0] != "docs/guide.md" {
		t.Fatalf("runtime context paths = %v", got)
	}
}
