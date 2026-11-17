package foundation_test

import (
	"context"
	"runtime/debug"
	"slices"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	yruntime "github.com/diasYuri/y/pkg/runtime"
	"github.com/diasYuri/y/pkg/tools"
)

// This consumer test compiles the actual pinned Charm stack and published Y
// together without starting a terminal, provider or external process.
func TestPublishedDependencies(t *testing.T) {
	editor := textarea.New()
	editor.SetValue("Opera")
	if editor.Value() != "Opera" {
		t.Fatal("textarea round trip failed")
	}
	_ = tea.NewView(lipgloss.NewStyle().Render(editor.Value()))
	_ = huh.NewInput().Title("Model")
	_ = harmonica.NewSpring(harmonica.FPS(60), 6, 0.2)
	harness, err := yruntime.NewHarness()
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := harness.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPublicToolCatalog(t *testing.T) {
	root := t.TempDir()
	registry := tools.NewRegistry()
	t.Cleanup(func() {
		if err := registry.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	for _, register := range []func() error{
		func() error { return tools.RegisterFilesystem(registry, tools.FilesystemOptions{WorkspaceRoot: root}) },
		func() error { return tools.RegisterShell(registry, tools.ShellOptions{WorkspaceRoot: root}) },
		func() error { return tools.RegisterGit(registry, tools.GitOptions{WorkspaceRoot: root}) },
		func() error { return tools.RegisterCodeSearch(registry, tools.CodeSearchOptions{WorkspaceRoot: root}) },
		func() error { return tools.RegisterLSP(registry, tools.LSPOptions{WorkspaceRoot: root}) },
	} {
		if err := register(); err != nil {
			t.Fatal(err)
		}
	}
	var names []string
	for _, descriptor := range registry.List() {
		names = append(names, descriptor.Name)
	}
	want := []string{
		"read_file", "write_file", "list_files", "search", "edit", "patch", "run_command",
		"git_status", "git_diff", "git_log", "git_branch", "git_checkout", "git_commit",
		"git_show", "git_history", "git_blame", "git_search_history",
		"search_text", "search_ast", "outline", "find_files",
		"definition", "references", "implementations", "hover", "type_definition",
		"callers", "callees", "document_symbols", "workspace_symbols",
	}
	slices.Sort(names)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("update SDK capability matrix: catalog %v, expected %v", names, want)
	}
}

func TestNoLocalReplacements(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("missing module build information")
	}
	for _, dep := range info.Deps {
		if dep.Replace != nil {
			t.Errorf("unexpected module replacement: %s", dep.Path)
		}
	}
}
