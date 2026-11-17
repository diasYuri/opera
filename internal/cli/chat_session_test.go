package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
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
	s := newChatSession(App{}, cfg, core.Options{}, nil)
	providers := s.LoginProviders()
	available := make(map[string]bool, len(providers))
	for _, provider := range providers {
		available[provider.ID] = true
	}
	for _, id := range []string{"anthropic", "deepseek", "google", "groq", "openai", "openrouter"} {
		if !available[id] {
			t.Fatalf("production registry is missing provider %q: %#v", id, providers)
		}
	}
	openai := s.loginMethod("openai")
	if openai == nil {
		t.Fatal("OpenAI login method was not registered")
	}
	openai.descriptor = nil // replace the interactive provider flow with this test double
	openai.provider = "openai-subscription"
	openai.login = func(_ context.Context, _ config.Resolved, notify func(execution.LoginNotice) error) error {
		return notify(execution.LoginNotice{URL: "https://example.test/authorize"})
	}
	var opened string
	s.openBrowser = func(_ context.Context, u string) error { opened = u; return errors.New("no browser installed") }
	var notices []execution.LoginNotice
	if err := s.Login(context.Background(), "openai", func(n execution.LoginNotice) error { notices = append(notices, n); return nil }); err != nil {
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
	s := newChatSession(App{}, cfg, core.Options{}, nil)
	first := s.Start(context.Background(), execution.Request{Prompt: "before login"})
	update := <-first.Updates
	if update.Error == nil || !strings.Contains(update.Error.Error(), "/login") || s.runtime != nil {
		t.Fatal("first-run prompt bypassed connection guidance")
	}
	openai := s.loginMethod("openai")
	if openai == nil {
		t.Fatal("OpenAI login method was not registered")
	}
	openai.descriptor = nil // replace the interactive provider flow with this test double
	openai.login = func(context.Context, config.Resolved, func(execution.LoginNotice) error) error {
		return context.Canceled
	}
	if err := s.Login(context.Background(), "openai", func(execution.LoginNotice) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, provider, model := s.SessionInfo()
	if provider != "" || model != "" {
		t.Fatal("cancelled login changed selection")
	}
	if err := s.Login(context.Background(), "unavailable", func(execution.LoginNotice) error { return nil }); err == nil {
		t.Fatal("unknown provider accepted")
	}
	providerCount := len(s.LoginProviders())
	s.methods = append(s.methods, loginMethod{LoginProvider: execution.LoginProvider{ID: "fixture", Name: "Fixture"}})
	if len(s.LoginProviders()) != providerCount+1 {
		t.Fatal("menu registry did not discover a newly registered provider")
	}
}

func (s *chatSession) loginMethod(id string) *loginMethod {
	for i := range s.methods {
		if s.methods[i].ID == id {
			return &s.methods[i]
		}
	}
	return nil
}
