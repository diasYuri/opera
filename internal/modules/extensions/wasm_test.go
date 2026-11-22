//go:build feature_wasm_ext

package extensions

import (
	"context"
	"errors"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/extensions/wasm/wasmtest"
	"github.com/diasYuri/y/pkg/tools"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func wasmFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	manifest := `id = "fixture.echo"
name = "Fixture"
version = "1.0.0"
api_version = "y.wasm.v1"
entry = "module.wasm"
[runtime]
memory_pages = 16
timeout_ms = 1000
[[tools]]
name = "echo"
input_schema = "{\"type\":\"object\",\"properties\":{}}"
output_schema = "{\"type\":\"object\",\"properties\":{\"content\":{\"type\":\"array\"}},\"required\":[\"content\"]}"
`
	if err := os.WriteFile(filepath.Join(dir, "extension.toml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	module := wasmtest.BuildABIToolModule([]byte(`{"ok":true,"payload":{"content":[{"type":"text","text":"wasm proof"}]}}`))
	if err := os.WriteFile(filepath.Join(dir, "module.wasm"), module, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWASMReloadPinsOldGenerationAndRollsBackInvalidCandidate(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	dir := wasmFixture(t)
	module := Module{ID: "fixture.echo", Directory: dir, Enabled: true}
	digest, err := ModuleDigest(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	module.Consent = digest
	settings := Settings{Version: 1, WASM: []Module{module}}
	if err := Save(cfg, settings); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry()
	defer func() { _ = reg.Close(ctx) }()
	m, err := New(ctx, cfg, reg, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close(ctx) }()
	view, err := m.Host.AcquireRuntimeView(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = view.Close() }()
	before := view.Info().Generation
	settings.WASM[0].Enabled = false
	if err := Save(cfg, settings); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() { close(started); done <- m.Reload(ctx) }()
	<-started
	select {
	case err := <-done:
		t.Fatalf("unload did not drain captured view: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	// Existing views must still call the handler until their scope is released.
	response, err := view.Tools().Handle(ctx, tools.ToolRequest{ID: "old", Name: "wasm_fixture_echo_echo", Arguments: []byte(`{}`)})
	if err != nil || len(response.Content) == 0 {
		t.Fatal("old view lost handler", err, response)
	}
	if view.Info().Generation != before {
		t.Fatal("captured generation changed")
	}
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, _, ok := reg.Get("wasm_fixture_echo_echo"); ok {
		t.Fatal("new generation kept unloaded tool")
	}
	settings.WASM[0].Enabled = true
	if err := os.WriteFile(filepath.Join(dir, "module.wasm"), []byte("broken module"), 0600); err != nil {
		t.Fatal(err)
	}
	settings.WASM[0].Consent, err = ModuleDigest(ctx, settings.WASM[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(cfg, settings); err != nil {
		t.Fatal(err)
	}
	current := m.Digest()
	if err := m.Reload(ctx); err == nil {
		t.Fatal("invalid candidate accepted")
	}
	if m.Digest() != current {
		t.Fatal("failed candidate replaced active settings")
	}
	if _, _, ok := reg.Get("wasm_fixture_echo_echo"); ok {
		t.Fatal("failed candidate leaked tool")
	}
}

func TestWASMReloadKeepsCommittedIdentityWhileOldViewDrains(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	module := Module{ID: "fixture.echo", Directory: wasmFixture(t), Enabled: true}
	var err error
	module.Consent, err = ModuleDigest(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	settings := Settings{Version: 1, WASM: []Module{module}}
	if err := Save(cfg, settings); err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	defer func() { _ = registry.Close(ctx) }()
	m, err := New(ctx, cfg, registry, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close(ctx) }()
	old, err := m.AcquireRuntimeView(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = old.Close() }()
	generation := m.ToolGeneration()
	if !strings.HasSuffix(old.Info().ID, generation) {
		t.Fatal("runtime view and checkpoint identity disagree")
	}
	settings.WASM[0].Enabled = false
	if err := Save(cfg, settings); err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err = m.Reload(wait)
	var transition *component.ReconcileError
	if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &transition) || !transition.Committed {
		t.Fatalf("expected a committed reconciliation waiting for a lease, got %v", err)
	}
	current, err := m.AcquireRuntimeView(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = current.Close() }()
	if m.ToolGeneration() == generation || !strings.HasSuffix(current.Info().ID, m.ToolGeneration()) ||
		current.Info().Generation == old.Info().Generation {
		t.Fatal("published view regressed to old settings after a committed drain deadline")
	}
	for _, tool := range current.Tools().List() {
		if tool.Name == "wasm_fixture_echo_echo" {
			t.Fatal("new view still advertises the retired tool")
		}
	}
	if _, err := old.Tools().Handle(ctx, tools.ToolRequest{ID: "old", Name: "wasm_fixture_echo_echo", Arguments: []byte(`{}`)}); err != nil {
		t.Fatalf("retired view lost its pinned handler: %v", err)
	}
}
