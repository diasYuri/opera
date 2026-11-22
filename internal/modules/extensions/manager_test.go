package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/opera/internal/modules/workspace"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"github.com/diasYuri/y/pkg/extensions/wasm"
	"github.com/diasYuri/y/pkg/tools"
)

func testConfig(t *testing.T) config.Resolved {
	t.Helper()
	root := t.TempDir()
	cfg, err := config.Load(config.Options{Paths: platform.Paths{Workspace: filepath.Join(root, "workspace"), Data: filepath.Join(root, "data")}})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
func TestSettingsConsentIsolationAndUnsupportedGrants(t *testing.T) {
	cfg := testConfig(t)
	settings := Settings{Version: 1, MCP: []Server{{Name: "test", Transport: "stdio", Command: "missing"}}}
	if err := Save(cfg, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	digest := ServerDigest(loaded.MCP[0])
	loaded.MCP[0].Args = []string{"changed"}
	if digest == ServerDigest(loaded.MCP[0]) {
		t.Fatal("arguments did not invalidate consent")
	}
	loaded.WASM = []Module{{ID: "test", Directory: t.TempDir(), Grants: []string{"filesystem"}}}
	if err := loaded.Validate(); err == nil {
		t.Fatal("unsupported host grant accepted")
	}
	cfg.Values["extensions.file"] = config.Value{Value: filepath.Join(cfg.Paths.Workspace, "extensions.toml")}
	if err := Save(cfg, settings); err == nil {
		t.Fatal("workspace-controlled activation accepted")
	}
}
func TestReloadRollbackAndSafePoint(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	registry := tools.NewRegistry()
	defer func() { _ = registry.Close(ctx) }()
	m, err := New(ctx, cfg, registry, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close(ctx) }()
	before := m.Digest()
	settings := m.Settings
	settings.Memory.Enabled = true
	if err := Save(cfg, settings); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(ctx); err == nil {
		t.Fatal("unsupported live memory replacement accepted")
	}
	if before != m.Digest() {
		t.Fatal("failed reload changed generation")
	}
	denied := errors.New("busy")
	m.safePoint = func(context.Context, string, func() error) error { return denied }
	if !errors.Is(m.Reload(ctx), denied) {
		t.Fatal("safe point bypassed")
	}
}
func TestReadBridgeUsesDurableCapturedRegistry(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	registry := tools.NewRegistry()
	defer func() { _ = registry.Close(ctx) }()
	guard := &execution.Guard{Queue: &workspace.Queue{}}
	if err := registry.AddExecutionMiddleware(guard.Middleware); err != nil {
		t.Fatal(err)
	}
	m, err := New(ctx, cfg, registry, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close(ctx) }()
	var calls int
	if err := registry.Add(tools.ToolDescriptor{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object"}`)}, tools.ToolHandlerFunc(func(context.Context, tools.ToolRequest) (tools.ToolResponse, error) {
		calls++
		return tools.ToolResponse{}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(tools.ToolDescriptor{Name: "wasm_test", InputSchema: json.RawMessage(`{"type":"object"}`)}, tools.ToolHandlerFunc(func(ctx context.Context, _ tools.ToolRequest) (tools.ToolResponse, error) {
		_, err := m.invokeReadTool(ctx, "test", "read_file", json.RawMessage(`{}`))
		return tools.ToolResponse{}, err
	})); err != nil {
		t.Fatal(err)
	}
	events := &observedOperations{InMemoryEventStore: agent.NewInMemoryEventStore(), names: map[string]int{}}
	runner := agent.NewAgentRunner(agenttest.NewScriptProvider(t, agenttest.ToolTurn("wasm_test", `{}`), agenttest.TextTurn("done")), registry, agent.WithRunnerStateStore(agent.NewInMemoryStateStore()), agent.WithRunnerOperationStore(events), agent.WithRunnerAgentOptions(m.AgentOptions()...))
	defer func() { _ = runner.Close(ctx) }()
	if _, err := runner.Run(ctx, agent.RunRequest{RunID: "bridge", Prompt: "read"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("read calls=%d", calls)
	}
	if events.names["read_file"] != 1 || events.names["wasm_test"] != 1 {
		t.Fatalf("durable operations missing: %v", events.names)
	}
	if _, err := m.invokeReadTool(ctx, "test", "read_file", json.RawMessage(`{}`)); err == nil {
		t.Fatal("bridge allowed missing durable view")
	}
	if _, err := m.invokeReadTool(ctx, "test", "write_file", json.RawMessage(`{}`)); err == nil {
		t.Fatal("bridge allowed a mutation")
	}
	if got := m.allowCapability(wasm.CapabilityRequest{ExtensionID: "test", Capability: wasm.CapYTools}); got != wasm.DecisionDeny {
		t.Fatal("ungranted host call accepted")
	}

}

type observedOperations struct {
	*agent.InMemoryEventStore
	names map[string]int
}

func (s *observedOperations) BeginOperation(ctx context.Context, op agent.OperationRecord) (agent.OperationRecord, bool, error) {
	s.names[op.ToolName]++
	return s.InMemoryEventStore.BeginOperation(ctx, op)
}

func TestConsentStoreRejectsParentSymlinkIntoWorkspace(t *testing.T) {
	cfg := testConfig(t)
	if err := os.MkdirAll(cfg.Paths.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(cfg.Paths.Workspace, alias); err != nil {
		t.Skip("symlinks unavailable", err)
	}
	cfg.Values["extensions.file"] = config.Value{Value: filepath.Join(alias, "consent.toml")}
	if err := Save(cfg, Settings{Version: 1}); err == nil {
		t.Fatal("parent symlink redirected consent into workspace")
	}
}
