package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	execmod "github.com/diasYuri/opera/internal/modules/execution"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"github.com/diasYuri/y/pkg/policy"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/providertest"
	yruntime "github.com/diasYuri/y/pkg/runtime"
	"github.com/diasYuri/y/pkg/search"
	"github.com/diasYuri/y/pkg/tools"
)

type namedProvider struct{ *agenttest.ScriptProvider }

func (namedProvider) ID() string { return "openai" }

type subscriptionNamedProvider struct{ *agenttest.ScriptProvider }

func (subscriptionNamedProvider) ID() string { return "openai-subscription" }

type scriptedSearch struct{ calls int }

func (*scriptedSearch) Capabilities() search.Capabilities { return search.Capabilities{} }
func (p *scriptedSearch) Search(_ context.Context, request search.Request) (search.Response, error) {
	p.calls++
	return search.Response{Results: []search.Result{{Title: "Verified source", URL: "https://example.com/fact", Snippet: request.Query}}}, nil
}

type capturedSearch struct{ request search.Request }

func (*capturedSearch) Capabilities() search.Capabilities {
	return search.Capabilities{AllowedDomains: true, ExcludedDomains: true, ContextSize: true, MaxResults: true, Country: true, Language: true, SafeSearch: true}
}
func (p *capturedSearch) Search(_ context.Context, request search.Request) (search.Response, error) {
	p.request = request
	return search.Response{Results: []search.Result{{Title: "Example", URL: "https://example.com", Snippet: "result"}}}, nil
}

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
	runtime, err := core.Open(ctx, cfg, core.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	execution := runtime.Service.Start(ctx, execmod.Request{Prompt: "start"})
	<-provider.entered
	execution.Cancel()
	var cancelled execmod.Result
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
	result, err := resumed.Service.Execute(ctx, execmod.Request{ID: cancelled.ID}, nil)
	if err != nil || result.ExitCode != 0 || result.Text != "recovered" {
		t.Fatalf("explicit recovery failed: %#v %v", result, err)
	}
}

func TestCloseCancelsAndDrainsApplicationBeforeStores(t *testing.T) {
	cfg := configuration(t)
	provider := &cancellableProvider{FakeProvider: providertest.NewFakeProvider(), entered: make(chan struct{}), block: true}
	runtime, err := core.Open(context.Background(), cfg, core.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	execution := runtime.Service.Start(context.Background(), execmod.Request{Prompt: "start"})
	<-provider.entered
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.Close(ctx); err != nil {
		t.Fatal("application did not drain before storage shutdown", err)
	}
	var result execmod.Result
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
	resumed, err := reopened.Service.Execute(context.Background(), execmod.Request{ID: result.ID}, nil)
	if err != nil || resumed.ExitCode != 0 {
		t.Fatal("shutdown checkpoint was not resumable", err)
	}
}

func TestUncertainEffectBlocksUntilExplicitReconciliation(t *testing.T) {
	cfg := configuration(t)
	ctx := context.Background()
	runtime := open(t, cfg, agenttest.TextTurn("done"))
	defer closeRuntime(t, runtime)
	first, err := runtime.Service.Execute(ctx, execmod.Request{Prompt: "hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, acquired, err := runtime.Service.Store.BeginOperation(ctx, agent.OperationRecord{RunID: first.ID, IdempotencyKey: "uncertain", ToolName: "write_file", ArgumentsHash: "hash"}); err != nil || !acquired {
		t.Fatal(err)
	}
	blocked, err := runtime.Service.Execute(ctx, execmod.Request{ID: first.ID}, nil)
	if err == nil || blocked.ExitCode != 4 {
		t.Fatal("uncertain operation did not block resume")
	}
	if err := runtime.Service.Store.CompleteOperation(ctx, "uncertain", tools.ToolResponse{Content: []tools.ContentBlock{{Type: tools.ContentText, Text: "user confirmed observed effect"}}}, nil); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Service.Execute(ctx, execmod.Request{ID: first.ID}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatal("explicit reconciliation did not unblock cached result", err)
	}
}
func open(t *testing.T, cfg config.Resolved, turns ...agenttest.ScriptedTurn) *core.Runtime {
	t.Helper()
	provider := namedProvider{agenttest.NewScriptProvider(t, turns...)}
	runtime, err := core.Open(context.Background(), cfg, core.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }, Credential: func(context.Context, string) (string, error) { return "fixture-secret-never-persist", nil }})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}
func closeRuntime(t *testing.T, runtime *core.Runtime) {
	t.Helper()
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWebSearchIsRegisteredAndRunsAfterApproval(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	searchProvider := &scriptedSearch{}
	provider := namedProvider{agenttest.NewScriptProvider(t,
		agenttest.ToolTurn("web_search", `{"query":"current company information"}`),
		agenttest.TextTurn("Verified [source](https://example.com/fact)."),
	)}
	runtime, err := core.Open(ctx, cfg, core.Options{
		ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil },
		Credential:      func(context.Context, string) (string, error) { return "fixture-secret-never-persist", nil },
		SearchProvider:  searchProvider,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)
	first, err := runtime.Service.Execute(ctx, execmod.Request{Prompt: "Search the web for current company information and cite the sources."}, nil)
	if err != nil || first.Approval == nil || first.Approval.Tool != "web_search" {
		t.Fatalf("web search did not enter the approval flow: %#v %v", first, err)
	}
	if searchProvider.calls != 0 {
		t.Fatal("external search ran before approval")
	}
	result, err := runtime.Service.Execute(ctx, execmod.Request{ID: first.ID, Approve: first.Approval.ID}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("approved search result: %#v %v", result, err)
	}
	if searchProvider.calls != 1 {
		t.Fatalf("search calls = %d, want 1", searchProvider.calls)
	}
}

func TestConfiguredBraveSearchDefersCredentialUntilAnApprovedCall(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["search.provider"] = config.Value{Value: "brave", Source: "user"}
	provider := namedProvider{agenttest.NewScriptProvider(t)}
	var resolved string
	runtime, err := core.Open(context.Background(), cfg, core.Options{
		ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil },
		Credential: func(_ context.Context, name string) (string, error) {
			resolved = name
			return "", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)
	if resolved != "" {
		t.Fatalf("search credential was resolved during activation: %q", resolved)
	}
	registry, err := yruntime.Resolve(runtime.Harness, yruntime.ToolRegistryKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range registry.List() {
		if descriptor.Name == "web_search" {
			if !descriptor.Sensitive {
				t.Fatal("web_search must retain the sensitive-operation approval policy")
			}
			_, err = registry.Handle(context.Background(), tools.ToolRequest{
				Name: tools.WebSearchToolName, Arguments: json.RawMessage(`{"query":"current company information"}`),
				Approval: &policy.ApprovalResolution{State: policy.ApprovalApproved},
			})
			if err == nil || !strings.Contains(err.Error(), "no Brave Search API key is available") {
				t.Fatalf("search without configured key returned %v", err)
			}
			if resolved != "brave-search" {
				t.Fatalf("approved search credential provider = %q", resolved)
			}
			return
		}
	}
	t.Fatal("configured Brave Search did not register web_search")
}

func TestConfiguredBraveSearchDoesNotFailActivationWithoutCredential(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["search.provider"] = config.Value{Value: "brave", Source: "user"}
	provider := namedProvider{agenttest.NewScriptProvider(t)}
	runtime, err := core.Open(context.Background(), cfg, core.Options{
		ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil },
		Credential:      func(context.Context, string) (string, error) { return "", nil },
	})
	if err != nil {
		t.Fatalf("configured search without a credential failed activation: %v", err)
	}
	closeRuntime(t, runtime)
}

func TestConfiguredWebSearchDefaultsReachProviderAndRequestValuesWin(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["provider"] = config.Value{Value: "openai-subscription", Source: "user"}
	cfg.Values["search.provider"] = config.Value{Value: "native", Source: "user"}
	cfg.Values["search.options.openai-subscription.max_results"] = config.Value{Value: 8, Source: "user-workspace"}
	cfg.Values["search.options.openai-subscription.country"] = config.Value{Value: "BR", Source: "user-workspace"}
	cfg.Values["search.options.openai-subscription.context_size"] = config.Value{Value: "low", Source: "user-workspace"}
	cfg.Values["search.options.openai-subscription.allowed_domains"] = config.Value{Value: "example.com", Source: "user-workspace"}
	cfg.Values["search.options.openai-subscription.excluded_domains"] = config.Value{Value: "private.example.com", Source: "user-workspace"}
	searchProvider := &capturedSearch{}
	providersCatalog, err := providercatalog.New(providercatalog.Entry{
		ID: "openai-subscription",
		Factory: func(context.Context, config.Resolved, func(context.Context, string) (string, error)) (providers.Provider, error) {
			return subscriptionNamedProvider{agenttest.NewScriptProvider(t)}, nil
		},
		SearchFactory: func(config.Resolved, func(context.Context, string) (string, error)) search.Provider {
			return searchProvider
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := core.Open(context.Background(), cfg, core.Options{ProviderCatalog: providersCatalog})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)
	registry, err := yruntime.Resolve(runtime.Harness, yruntime.ToolRegistryKey)
	if err != nil {
		t.Fatal(err)
	}
	call := func(arguments string) {
		t.Helper()
		_, err := registry.Handle(context.Background(), tools.ToolRequest{Name: tools.WebSearchToolName, Arguments: json.RawMessage(arguments), Approval: &policy.ApprovalResolution{State: policy.ApprovalApproved}})
		if err != nil {
			t.Fatal(err)
		}
	}
	call(`{"query":"current information"}`)
	got := searchProvider.request
	if got.MaxResults != 8 || got.Country != "BR" || got.ContextSize != search.ContextSizeLow || strings.Join(got.AllowedDomains, ",") != "example.com" || strings.Join(got.ExcludedDomains, ",") != "private.example.com" {
		t.Fatalf("configured defaults were not applied through composition: %#v", got)
	}
	call(`{"query":"current information","max_results":3,"country":"US","context_size":"high","allowed_domains":["docs.example.com"]}`)
	got = searchProvider.request
	if got.MaxResults != 3 || got.Country != "US" || got.ContextSize != search.ContextSizeHigh || strings.Join(got.AllowedDomains, ",") != "docs.example.com" || strings.Join(got.ExcludedDomains, ",") != "private.example.com" {
		t.Fatalf("request values did not override matching defaults: %#v", got)
	}
}

func TestCodexSearchResolvesItsCredentialOnlyAfterApproval(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	cfg.Values["provider"] = config.Value{Value: "openai-subscription", Source: "test"}
	cfg.Values["model"] = config.Value{Value: "fixture-codex-model", Source: "test"}
	provider := subscriptionNamedProvider{agenttest.NewScriptProvider(t,
		agenttest.ToolTurn("web_search", `{"query":"current company information"}`),
		agenttest.TextTurn("Search credential was checked after approval."),
	)}
	credentialLookups := 0
	runtime, err := core.Open(ctx, cfg, core.Options{
		ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil },
		Credential: func(_ context.Context, provider string) (string, error) {
			credentialLookups++
			if provider != "openai-subscription" {
				t.Errorf("unexpected native search credential provider %q", provider)
			}
			// The agent resolves its inference credential once per model turn;
			// the second lookup is the hosted search after approval.
			if credentialLookups == 2 {
				return "", nil
			}
			return "fixture-subscription-token", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)
	first, err := runtime.Service.Execute(ctx, execmod.Request{Prompt: "Search the web for current company information."}, nil)
	if err != nil || first.Approval == nil || first.Approval.Tool != "web_search" {
		t.Fatalf("native web search did not enter approval: %#v %v", first, err)
	}
	if credentialLookups != 1 {
		t.Fatalf("credential lookups before approval = %d, want only the agent inference credential", credentialLookups)
	}
	_, err = runtime.Service.Execute(ctx, execmod.Request{ID: first.ID, Approve: first.Approval.ID}, nil)
	if err != nil {
		t.Fatalf("approved search turn failed: %v", err)
	}
	if credentialLookups != 3 {
		t.Fatalf("credential lookups after approved search = %d, want inference + hosted search + next inference", credentialLookups)
	}
}

func TestNativeSearchCanBeDisabledExplicitly(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["provider"] = config.Value{Value: "openai-subscription", Source: "test"}
	cfg.Values["model"] = config.Value{Value: "fixture-codex-model", Source: "test"}
	cfg.Values["search.provider"] = config.Value{Value: "disabled", Source: "test"}
	runtime, err := core.Open(context.Background(), cfg, core.Options{
		ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
			return subscriptionNamedProvider{agenttest.NewScriptProvider(t)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)
	registry, err := yruntime.Resolve(runtime.Harness, yruntime.ToolRegistryKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range registry.List() {
		if descriptor.Name == "web_search" {
			t.Fatal("web_search remained available while explicitly disabled")
		}
	}
}

func TestProjectProviderSelectionCannotEnableNativeSearchByItself(t *testing.T) {
	for _, test := range []struct {
		name      string
		selection string
		want      bool
	}{
		{name: "automatic", want: false},
		{name: "user opt-in", selection: "native", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := configuration(t)
			cfg.Values["provider"] = config.Value{Value: "openai-subscription", Source: "project"}
			cfg.Values["model"] = config.Value{Value: "fixture-codex-model", Source: "project"}
			if test.selection != "" {
				cfg.Values["search.provider"] = config.Value{Value: test.selection, Source: "user"}
			}
			runtime, err := core.Open(context.Background(), cfg, core.Options{
				ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
					return subscriptionNamedProvider{agenttest.NewScriptProvider(t)}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer closeRuntime(t, runtime)
			registry, err := yruntime.Resolve(runtime.Harness, yruntime.ToolRegistryKey)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, descriptor := range registry.List() {
				found = found || descriptor.Name == "web_search"
			}
			if found != test.want {
				t.Fatalf("web_search availability = %v, want %v", found, test.want)
			}
		})
	}
}

func TestVerticalApprovalCheckpointAndResume(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	one := open(t, cfg, agenttest.TextAndToolTurn("Preparing change", "write_file", `{"path":"result.txt","content":"verified"}`))
	var text strings.Builder
	first, err := one.Service.Execute(ctx, execmod.Request{Prompt: "write the result"}, func(event execmod.Event) error { text.WriteString(event.Text); return nil })
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
	second, err := two.Service.Execute(ctx, execmod.Request{ID: first.ID, Approve: first.Approval.ID}, nil)
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
	replayed, err := two.Service.Execute(ctx, execmod.Request{ID: first.ID}, nil)
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
	first, err := one.Service.Execute(ctx, execmod.Request{Prompt: "change"}, nil)
	if err != nil || first.Approval == nil {
		t.Fatal("missing approval", err)
	}
	if err := os.WriteFile(path, []byte("concurrent change"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := one.Service.Execute(ctx, execmod.Request{ID: first.ID, Approve: first.Approval.ID}, nil)
	if err != nil || changed.ExitCode != 3 || changed.Approval.ID == first.Approval.ID {
		t.Fatal("stale diff was approved", err)
	}
	denied, err := one.Service.Execute(ctx, execmod.Request{ID: first.ID, Deny: changed.Approval.ID}, nil)
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
	_, err := core.Open(context.Background(), cfg, core.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		return nil, errors.New("fixture startup failure")
	}})
	if err == nil {
		t.Fatal("startup failure swallowed")
	}
	runtime := open(t, cfg, agenttest.TextTurn("done"))
	defer closeRuntime(t, runtime)
	if _, err := runtime.Service.Execute(context.Background(), execmod.Request{Prompt: "hello"}, nil); err != nil {
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
	_, err := core.Open(context.Background(), cfg, core.Options{
		ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil },
		ToolPacks: []core.ToolPack{{ID: "failing-resource", Register: func(_ context.Context, registry *tools.Registry, _ core.ToolPackContext) error {
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
	runtime, err := core.Open(context.Background(), cfg, core.Options{Inspection: true, ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
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
	runtime, err := core.Open(context.Background(), cfg, core.Options{ProviderCatalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)
	result, err := runtime.Service.Execute(context.Background(), execmod.Request{Prompt: "say hello"}, nil)
	if err != nil || result.Text != "answer from the custom provider" {
		t.Fatalf("custom provider result = %#v, %v", result, err)
	}
}

func TestCancelledStartupDoesNotCreateProvider(t *testing.T) {
	cfg := configuration(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	created := false
	_, err := core.Open(ctx, cfg, core.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
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
	runtime, err := core.Open(context.Background(), cfg, core.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
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

func TestProviderFactoryErrorClosesPartialProvider(t *testing.T) {
	cfg := configuration(t)
	providerClosed := 0
	provider := &observedCloseProvider{FakeProvider: providertest.NewFakeProvider(), closed: &providerClosed, order: &[]string{}}
	failure := errors.New("provider setup failed")
	_, err := core.Open(context.Background(), cfg, core.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, failure }})
	if !errors.Is(err, failure) {
		t.Fatalf("Open error = %v, want factory error", err)
	}
	if providerClosed != 1 {
		t.Fatalf("partial provider close count = %d, want 1", providerClosed)
	}
}

func TestContextCommandUpdatesRequestSourceAndResume(t *testing.T) {
	ctx := context.Background()
	cfg := configuration(t)
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "guide.txt"), []byte("workspace guidance"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := open(t, cfg, agenttest.TextTurn("done"))
	if _, err := runtime.Command(ctx, "/context guide.txt"); err != nil {
		t.Fatal(err)
	}
	paths := runtime.Service.ContextPathSource.Current()
	if len(paths) != 1 || paths[0] != "guide.txt" {
		t.Fatalf("context command paths = %v", paths)
	}
	result, err := runtime.Service.Execute(ctx, execmod.Request{Prompt: "hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, runtime)
	resumed, err := core.Open(ctx, cfg, core.Options{ResumeID: result.ID, ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
		return namedProvider{agenttest.NewScriptProvider(t)}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, resumed)
	paths = resumed.Service.ContextPathSource.Current()
	if len(paths) != 1 || paths[0] != "guide.txt" {
		t.Fatalf("resumed context paths = %v", paths)
	}
}
