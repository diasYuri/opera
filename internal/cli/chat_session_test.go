package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/harness"
	"github.com/diasYuri/opera/internal/platform"
)

func sessionConfig(t *testing.T) config.Resolved {
	t.Helper()
	root := t.TempDir()
	cfg, err := config.Load(config.Options{Paths: platform.Paths{Workspace: root, Data: filepath.Join(root, "data"), Credentials: filepath.Join(root, "data", "credentials")}})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestChatLoginRegistryAndSelection(t *testing.T) {
	cfg := sessionConfig(t)
	cfg.Values["provider"] = config.Value{Value: "openai-compatible"}
	cfg.Values["model"] = config.Value{Value: "old-model"}
	cfg.Values["base_url"] = config.Value{Value: "https://old-provider.test"}
	s := newChatSession(App{}, cfg, harness.Options{}, nil)
	if providers := s.LoginProviders(); len(providers) != 1 || providers[0].ID != "openai" || providers[0].Name != "OpenAI" {
		t.Fatal("production registry must offer only OpenAI", providers)
	}
	s.methods[0].login = func(_ context.Context, _ config.Resolved, notify func(app.LoginNotice) error) error {
		return notify(app.LoginNotice{URL: "https://example.test/authorize"})
	}
	var opened string
	s.openBrowser = func(_ context.Context, u string) error { opened = u; return errors.New("no browser installed") }
	var notices []app.LoginNotice
	if err := s.Login(context.Background(), "openai", func(n app.LoginNotice) error { notices = append(notices, n); return nil }); err != nil {
		t.Fatal(err)
	}
	if opened == "" || len(notices) != 2 || notices[0].URL == "" || !strings.Contains(notices[1].Message, "Open the authorization link") {
		t.Fatal("browser fallback lost authorization URL")
	}
	_, provider, model := s.SessionInfo()
	if provider != "openai-subscription" || model != "" || s.cfg.String("base_url") != "" {
		t.Fatal("login reused another provider's model or endpoint")
	}
	if _, err := s.Command(context.Background(), "/model openai-subscription account-model"); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(config.Options{Paths: cfg.Paths})
	if err != nil || loaded.String("provider") != "openai-subscription" || loaded.String("model") != "account-model" {
		t.Fatal("selected account model was not saved", err)
	}
	if cfg.String("provider") != "openai-compatible" {
		t.Fatal("session mutated caller's configuration")
	}
}

func TestChatLoginFailurePreservesSelectionAndFirstRunNeedsNoRuntime(t *testing.T) {
	cfg := sessionConfig(t)
	s := newChatSession(App{}, cfg, harness.Options{}, nil)
	first := s.Start(context.Background(), app.Request{Prompt: "before login"})
	update := <-first.Updates
	if update.Error == nil || !strings.Contains(update.Error.Error(), "/login") || s.runtime != nil {
		t.Fatal("first-run prompt bypassed connection guidance")
	}
	s.methods[0].login = func(context.Context, config.Resolved, func(app.LoginNotice) error) error { return context.Canceled }
	if err := s.Login(context.Background(), "openai", func(app.LoginNotice) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, provider, model := s.SessionInfo()
	if provider != "" || model != "" {
		t.Fatal("cancelled login changed selection")
	}
	if err := s.Login(context.Background(), "unavailable", func(app.LoginNotice) error { return nil }); err == nil {
		t.Fatal("unknown provider accepted")
	}
	s.methods = append(s.methods, loginMethod{LoginProvider: app.LoginProvider{ID: "fixture", Name: "Fixture"}})
	if len(s.LoginProviders()) != 2 {
		t.Fatal("menu registry did not discover a newly registered provider")
	}
}
