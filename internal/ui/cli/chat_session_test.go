package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/opera/internal/adapters/credentials"
	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
	ui "github.com/diasYuri/opera/internal/ui/tui"
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
	s.cfg.Values["credentials.store"] = config.Value{Value: "file"}
	s.app.OpenStore = credentials.Open
	openai.descriptor = &providercatalog.LoginDescriptor{ID: "openai", Name: "OpenAI", Methods: []providercatalog.LoginMethod{{ID: "fixture", Name: "Fixture", Kind: "oauth", ProviderID: "openai-subscription", Authenticator: loginHandler{run: func(_ context.Context, io providercatalog.AuthIO) error {
		return io.Notify(context.Background(), providercatalog.LoginNotice{URL: "https://example.test/authorize"})
	}}}}}
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
	s.cfg.Values["credentials.store"] = config.Value{Value: "file"}
	s.app.OpenStore = credentials.Open
	openai.descriptor = &providercatalog.LoginDescriptor{ID: "openai", Name: "OpenAI", Methods: []providercatalog.LoginMethod{{ID: "fixture", Name: "Fixture", Kind: "oauth", ProviderID: "openai-subscription", Authenticator: loginHandler{run: func(context.Context, providercatalog.AuthIO) error { return context.Canceled }}}}}
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

func TestWebSearchAuthenticationOptionsFollowProviderCatalog(t *testing.T) {
	cfg := sessionConfig(t)
	s := newChatSession(App{}, cfg, core.Options{}, nil)
	providers := s.WebSearchProviders()
	options := make(map[string][]ui.WebSearchAuthOption, len(providers))
	for _, provider := range providers {
		options[provider.ID] = provider.AuthOptions
	}
	if len(options["openai-subscription"]) != 1 || options["openai-subscription"][0].MethodID != "chatgpt" {
		t.Fatalf("Codex authentication methods = %#v", options["openai-subscription"])
	}
	if len(options["anthropic"]) != 2 {
		t.Fatalf("Claude authentication methods = %#v", options["anthropic"])
	}
	var claudeOAuth, claudeAPIKey bool
	for _, option := range options["anthropic"] {
		claudeOAuth = claudeOAuth || option.MethodID == "claude"
		claudeAPIKey = claudeAPIKey || option.MethodID == "api-key" && option.Kind == "api-key"
	}
	if !claudeOAuth || !claudeAPIKey {
		t.Fatalf("Claude sign-in methods missing: %#v", options["anthropic"])
	}
	if len(options["brave"]) != 1 || options["brave"][0].CredentialProviderID != "brave-search" || options["brave"][0].Kind != "api-key" {
		t.Fatalf("Brave authentication methods = %#v", options["brave"])
	}
}

func TestWebSearchAPIKeyLoginStoresSecretWithoutChangingModel(t *testing.T) {
	cfg := sessionConfig(t)
	cfg.Values["provider"] = config.Value{Value: "anthropic", Source: "user-workspace"}
	cfg.Values["model"] = config.Value{Value: "claude-sonnet-test", Source: "user-workspace"}
	cfg.Values["credentials.store"] = config.Value{Value: "file", Source: "user-workspace"}
	s := newChatSession(App{
		OpenStore: credentials.Open,
		Env:       func(string) (string, bool) { return "", false },
	}, cfg, core.Options{}, nil)
	options := s.WebSearchAuthOptions("brave")
	if len(options) != 1 {
		t.Fatalf("Brave API-key option = %#v", options)
	}
	notify := func(notice execution.LoginNotice) error {
		if notice.Prompt != nil {
			notice.Prompt.Response <- "fixture-brave-api-key"
		}
		return nil
	}
	if err := s.LoginWebSearch(context.Background(), options[0], notify); err != nil {
		t.Fatal(err)
	}
	store, err := credentials.Open("file", cfg.Paths.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := store.Get("brave-search.default")
	if err != nil || secret != "fixture-brave-api-key" {
		t.Fatalf("API key was not saved to the credential store: present=%t, err=%v", secret != "", err)
	}
	if s.cfg.String("provider") != "anthropic" || s.cfg.String("model") != "claude-sonnet-test" {
		t.Fatalf("search authentication changed the active model: %s/%s", s.cfg.String("provider"), s.cfg.String("model"))
	}
}

type loginHandler struct {
	run func(context.Context, providercatalog.AuthIO) error
}

func (loginHandler) Supports(string) bool { return true }
func (h loginHandler) Authenticate(ctx context.Context, _ string, io providercatalog.AuthIO) (providercatalog.AuthResult, error) {
	return providercatalog.AuthResult{}, h.run(ctx, io)
}
func (loginHandler) Credential(context.Context, providercatalog.AuthIO) (string, error) {
	return "", nil
}

func (s *chatSession) loginMethod(id string) *loginMethod {
	for i := range s.methods {
		if s.methods[i].ID == id {
			return &s.methods[i]
		}
	}
	return nil
}
