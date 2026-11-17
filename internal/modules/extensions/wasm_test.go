//go:build feature_wasm_ext

package extensions

import (
	"context"
	"github.com/diasYuri/y/pkg/extensions/wasm/wasmtest"
	"github.com/diasYuri/y/pkg/tools"
	"os"
	"path/filepath"
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
