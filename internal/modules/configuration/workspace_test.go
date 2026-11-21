package config_test

import (
	"path/filepath"
	"testing"

	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/modules/configuration"
)

func TestWorkspaceOnboardingIsScopedAndUserOwned(t *testing.T) {
	root := t.TempDir()
	paths := platform.Paths{Workspace: root, Data: filepath.Join(root, "data"), UserConfig: filepath.Join(root, "missing-user"), ProjectConfig: filepath.Join(root, "missing-project")}
	if err := config.SaveWorkspaceSelection(paths, "openai", "fixture", "trusted"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{Paths: paths})
	if err != nil || cfg.String("policy") != "trusted" || cfg.Values["policy"].Source != "user-workspace" {
		t.Fatal("onboarding selection not loaded", err)
	}
	paths.Workspace = filepath.Join(root, "other")
	other, err := config.Load(config.Options{Paths: paths})
	if err != nil || other.String("policy") != "ask" {
		t.Fatal("workspace trust leaked", err)
	}
}

func TestWorkspaceWebSearchOptionsPersistOutsideTheProject(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	paths := platform.Paths{Workspace: workspace, Data: filepath.Join(root, "data"), UserConfig: filepath.Join(root, "missing-user"), ProjectConfig: filepath.Join(workspace, ".opera", "config.toml")}
	if err := config.SaveWorkspacePreference(paths, "search.options.openai-subscription.context_size", "high"); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveWorkspacePreference(paths, "search.options.openai-subscription.allowed_domains", "OpenAI.com, docs.openai.com"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defaults := cfg.WebSearchDefaults("openai-subscription")
	if defaults.ContextSize != "high" || len(defaults.AllowedDomains) != 2 || defaults.AllowedDomains[0] != "openai.com" || defaults.AllowedDomains[1] != "docs.openai.com" || cfg.Values["search.options.openai-subscription.context_size"].Source != "user-workspace" {
		t.Fatalf("workspace search preferences did not roundtrip: %#v", defaults)
	}
}
