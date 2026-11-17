//go:build feature_wasm_ext

package cli_test

import (
	"github.com/diasYuri/y/pkg/extensions/wasm/wasmtest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
func TestWASMLoadCallUnloadAndCapabilityRefusal(t *testing.T) {
	f := capabilitiesFixture(t)
	dir := wasmFixture(t)
	if code := f.app.Run([]string{"extensions", "add", "fixture.echo", dir, "--format=jsonl"}); code != 0 {
		t.Fatal(code, f.err.String())
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"extensions", "load", "fixture.echo", "--format=jsonl"}); code != 3 {
		t.Fatal(code, f.err.String(), f.out.String())
	}
	approve(t, f, executionResult(t, f.out.String()))
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"tools", "call", "wasm_fixture_echo_echo", `{}`, "--format=jsonl"}); code != 3 {
		t.Fatal(code, f.err.String(), f.out.String())
	}
	result := approve(t, f, executionResult(t, f.out.String()))
	if !strings.Contains(result.Text, "wasm proof") {
		t.Fatal(result.Text)
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"extensions", "unload", "fixture.echo", "--policy=trusted", "--format=jsonl"}); code != 0 {
		t.Fatal(code, f.err.String(), f.out.String())
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"tools", "call", "wasm_fixture_echo_echo", `{}`, "--format=jsonl"}); code == 0 {
		t.Fatal("unloaded tool still available")
	}
}

func TestWASMTrapIsFailureAndMissingModuleCanBeUnloaded(t *testing.T) {
	f := capabilitiesFixture(t)
	dir := wasmFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "module.wasm"), wasmtest.BuildABITrappingModule(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"extensions", "add", "fixture.echo", dir}, {"extensions", "load", "fixture.echo", "--policy=trusted"}} {
		f.out.Reset()
		f.err.Reset()
		if code := f.app.Run(args); code != 0 {
			t.Fatal(code, f.err.String(), f.out.String())
		}
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"tools", "call", "wasm_fixture_echo_echo", `{}`, "--policy=trusted", "--format=jsonl"}); code == 0 {
		t.Fatal("trap reported success")
	}
	if err := os.Remove(filepath.Join(dir, "module.wasm")); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"extensions", "unload", "fixture.echo", "--policy=trusted", "--format=jsonl"}); code != 0 {
		t.Fatal("cannot disable broken module", code, f.err.String(), f.out.String())
	}
}
