package core_test

import (
	"context"
	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodingEditApprovalAndShell(t *testing.T) {
	cfg := configuration(t)
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "sample.txt"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	r := open(t, cfg, agenttest.ToolTurn("edit", `{"path":"sample.txt","old_text":"before","new_text":"after"}`), agenttest.TextTurn("edited"))
	defer closeRuntime(t, r)
	result, err := r.Service.Execute(context.Background(), app.Request{Prompt: "edit"}, nil)
	if err != nil || result.ExitCode != 3 {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = r.Service.Execute(context.Background(), app.Request{ID: result.ID, Approve: result.Approval.ID}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.Paths.Workspace, "sample.txt"))
	if err != nil || string(data) != "after" {
		t.Fatal(string(data), err)
	}
}
func TestCodingShellRequiresApproval(t *testing.T) {
	cfg := configuration(t)
	r := open(t, cfg, agenttest.ToolTurn("run_command", `{"command":"go","args":["version"]}`), agenttest.TextTurn("checked"))
	defer closeRuntime(t, r)
	result, err := r.Service.Execute(context.Background(), app.Request{Prompt: "check"}, nil)
	if err != nil || result.Approval == nil || result.Approval.Tool != "run_command" {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = r.Service.Execute(context.Background(), app.Request{ID: result.ID, Approve: result.Approval.ID}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestContextCommandsAndSafeModelSwitch(t *testing.T) {
	cfg := configuration(t)
	r := open(t, cfg)
	defer func() { closeRuntime(t, r) }()
	if _, err := r.Command(context.Background(), "/model openai another-model"); err != nil {
		t.Fatal(err)
	}
	if r.Service.Model != "another-model" {
		t.Fatal("model not applied")
	}
	if _, err := r.Command(context.Background(), "/thinking invalid"); err == nil {
		t.Fatal("invalid thinking accepted")
	}
	if _, err := r.Command(context.Background(), "/context ../outside"); err == nil {
		t.Fatal("outside context accepted")
	}
}

func TestSemanticFallbackAndGitInspection(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cfg := configuration(t)
	cfg.Values["policy"] = config.Value{Value: "trusted", Source: "test"}
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "sample.go"), []byte("package sample\nfunc Run() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := open(t, cfg, agenttest.ToolTurn("definition", `{"path":"sample.go","line":2,"column":6}`), agenttest.ToolTurn("search", `{"query":"Run"}`), agenttest.ToolTurn("read_file", `{"path":"sample.go"}`), agenttest.ToolTurn("git_diff", `{}`), agenttest.TextTurn("inspected with fallback"))
	defer closeRuntime(t, r)
	result, err := r.Service.Execute(context.Background(), app.Request{Prompt: "Inspect the symbol; use file search if semantic navigation is unavailable"}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	operations, err := r.Service.Store.Operations(context.Background(), result.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, operation := range operations {
		seen[operation.ToolName] = true
	}
	for _, name := range []string{"definition", "search", "read_file", "git_diff"} {
		if !seen[name] {
			t.Fatal("tool did not pass durable operation store", name)
		}
	}
}

func TestReadOnlyRejectsShellWithoutEffect(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["policy"] = config.Value{Value: "read-only", Source: "test"}
	r := open(t, cfg, agenttest.ToolTurn("run_command", `{"command":"echo unsafe > effect.txt","shell":true}`), agenttest.TextTurn("denied"))
	defer closeRuntime(t, r)
	result, err := r.Service.Execute(context.Background(), app.Request{Prompt: "run"}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.Workspace, "effect.txt")); !os.IsNotExist(err) {
		t.Fatal("shell bypassed read-only policy", err)
	}
}

func TestPatchApprovalIncludesNestedFilePrecondition(t *testing.T) {
	cfg := configuration(t)
	if err := os.Mkdir(filepath.Join(cfg.Paths.Workspace, "b"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfg.Paths.Workspace, "b", "sample.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := open(t, cfg, agenttest.ToolTurn("patch", `{"patch":"--- a/b/sample.txt\n+++ b/b/sample.txt\n@@ -1 +1 @@\n-old\n+new\n"}`))
	defer closeRuntime(t, r)
	first, err := r.Service.Execute(context.Background(), app.Request{Prompt: "patch"}, nil)
	if err != nil || first.Approval == nil {
		t.Fatalf("%+v %v", first, err)
	}
	if !strings.Contains(first.Approval.Path, filepath.Join("b", "sample.txt")) {
		t.Fatal(first.Approval.Path)
	}
	if err := os.WriteFile(path, []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := r.Service.Execute(context.Background(), app.Request{ID: first.ID, Approve: first.Approval.ID}, nil)
	if err != nil || second.Approval == nil || second.Approval.ID == first.Approval.ID {
		t.Fatalf("stale patch approved: %+v %v", second, err)
	}
}
