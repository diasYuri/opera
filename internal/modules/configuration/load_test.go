package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/modules/configuration"
)

func fixture(t *testing.T, user, project string) config.Options {
	t.Helper()
	dir := t.TempDir()
	paths := platform.Paths{UserConfig: filepath.Join(dir, "user.toml"), ProjectConfig: filepath.Join(dir, "project.toml")}
	for path, text := range map[string]string{paths.UserConfig: user, paths.ProjectConfig: project} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return config.Options{Paths: paths}
}

func TestPrecedenceAndProvenance(t *testing.T) {
	opts := fixture(t, "provider='openai'\nmodel='user'\npolicy='trusted'\n[ui]\nno_motion=true", "model='project'\npolicy='read-only'")
	env := map[string]string{"OPERA_MODEL": "environment", "OPERA_UI_NO_MOTION": "false"}
	opts.Env = func(key string) (string, bool) { v, ok := env[key]; return v, ok }
	opts.Flags = map[string]string{"model": "flag"}
	cfg, err := config.Load(opts)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]config.Value{
		"provider": {Value: "openai", Source: "user"}, "model": {Value: "flag", Source: "flag"},
		"policy": {Value: "read-only", Source: "project"}, "ui.no_motion": {Value: false, Source: "environment"},
		"agents.max_concurrent": {Value: 4, Source: "default"},
	} {
		if got := cfg.Values[key]; got != want {
			t.Errorf("%s: got %#v, want %#v", key, got, want)
		}
	}
	delete(opts.Flags, "model")
	cfg, err = config.Load(opts)
	if err != nil || cfg.String("model") != "environment" {
		t.Fatalf("environment should override project: %v", err)
	}
	delete(env, "OPERA_MODEL")
	cfg, err = config.Load(opts)
	if err != nil || cfg.String("model") != "project" {
		t.Fatalf("project should override user: %v", err)
	}
}

func TestEmptyAndFalseAreExplicit(t *testing.T) {
	opts := fixture(t, "model='user'\n[ui]\naccessible=true", "model=''\n[ui]\naccessible=false")
	cfg, err := config.Load(opts)
	if err != nil || cfg.String("model") != "" || cfg.Bool("ui.accessible") {
		t.Fatalf("empty/false lost: %#v, %v", cfg, err)
	}
}

func TestProjectCannotElevateOrSelectStore(t *testing.T) {
	for _, project := range []string{"policy='trusted'", "[credentials]\nstore='file'"} {
		opts := fixture(t, "", project)
		if _, err := config.Load(opts); err == nil {
			t.Errorf("accepted privileged project configuration")
		}
	}
	if _, err := config.Load(fixture(t, "policy='read-only'", "policy='ask'")); err == nil {
		t.Fatal("project elevated read-only")
	}
	opts := fixture(t, "", "")
	opts.Flags = map[string]string{"policy": "trusted"}
	if cfg, err := config.Load(opts); err != nil || cfg.String("policy") != "trusted" {
		t.Fatalf("explicit user flag should be allowed: %v", err)
	}
}

func TestInvalidConfigurationNeverEchoesValues(t *testing.T) {
	const secret = "private-sentinel-do-not-print"
	for _, text := range []string{
		"api_key='" + secret + "'", "model='" + secret,
		"[unknown]\nvalue='" + secret + "'", "[unknown]", "[limits]\ncontext_tokens='" + secret + "'",
		"[ui]\naccessible='true'", "[agents]\nmax_concurrent=0", "[limits]\nprocess_timeout='0s'",
		"[limits]\nshutdown_timeout='25h'", "model=['" + secret + "']", "[ui]\naccessible=1",
	} {
		_, err := config.Load(fixture(t, text, ""))
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("expected safe validation error, got %v", err)
		}
	}
	for _, flags := range []map[string]string{{"provider": "bad\x1b[31m"}, {"model": "bad\x1b[31m"}, {"secret": secret}, {"agents.max_depth": "-1"}, {"ui.no_motion": secret}} {
		opts := fixture(t, "", "")
		opts.Flags = flags
		if _, err := config.Load(opts); err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("expected safe flag error, got %v", err)
		}
	}
}

func TestProviderIDIsResolvedByCompositionCatalog(t *testing.T) {
	cfg, err := config.Load(fixture(t, "provider='custom-provider'", ""))
	if err != nil || cfg.String("provider") != "custom-provider" {
		t.Fatalf("custom provider ID should pass generic config validation: cfg=%v err=%v", cfg.String("provider"), err)
	}
}

func TestMissingOptionalFilesAndLimits(t *testing.T) {
	opts := config.Options{Paths: platform.Paths{UserConfig: filepath.Join(t.TempDir(), "missing"), ProjectConfig: filepath.Join(t.TempDir(), "missing")}}
	cfg, err := config.Load(opts)
	if err != nil || cfg.Int("agents.max_concurrent") != 4 || cfg.String("policy") != "ask" {
		t.Fatalf("defaults: %#v, %v", cfg, err)
	}
	if err := os.WriteFile(opts.Paths.UserConfig, []byte(strings.Repeat(" ", (1<<20)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(opts); err == nil {
		t.Fatal("oversized configuration accepted")
	}
}
