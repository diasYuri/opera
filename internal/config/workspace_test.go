package config_test

import (
	"path/filepath"
	"testing"

	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/platform"
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
