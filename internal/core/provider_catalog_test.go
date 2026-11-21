package core_test

import (
	"context"
	"testing"

	"github.com/diasYuri/opera/internal/core"
	config "github.com/diasYuri/opera/internal/modules/configuration"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
)

func TestDefaultCatalogRoutesChatGPTLoginByTargetProvider(t *testing.T) {
	catalog, err := core.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}

	var openAI *providercatalog.LoginDescriptor
	for _, item := range catalog.Logins() {
		descriptor := item
		if descriptor.ID == "openai" {
			openAI = &descriptor
		}
		if descriptor.ID == "openai-subscription" {
			t.Fatal("subscription login duplicated the OpenAI login group")
		}
	}
	if openAI == nil || len(openAI.Methods) != 2 {
		t.Fatalf("OpenAI login descriptor = %#v", openAI)
	}
	var chatGPTMethod bool
	for _, method := range openAI.Methods {
		if method.ID == "chatgpt" {
			chatGPTMethod = method.ProviderID == "openai-subscription" && method.Authenticator != nil
		}
	}
	if !chatGPTMethod {
		t.Fatal("ChatGPT method is missing its subscription authenticator")
	}
	if catalog.Authenticator("openai-subscription") == nil {
		t.Fatal("subscription authenticator was not indexed by target provider")
	}
	if catalog.Authenticator("openai") != nil {
		t.Fatal("OpenAI API login unexpectedly resolved to the subscription authenticator")
	}
}

func TestDefaultCatalogProvidesSearchForCodexAndClaudeOnlyAtOfficialEndpoints(t *testing.T) {
	catalog, err := core.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Resolved{Values: map[string]config.Value{"model": {Value: "fixture-model", Source: "test"}}}
	lookup := func(context.Context, string) (string, error) { return "fixture-token", nil }
	for _, provider := range []string{"openai-subscription", "anthropic"} {
		search := catalog.SearchProvider(provider, cfg, lookup)
		if search == nil {
			t.Fatalf("native search provider missing for %q", provider)
		}
		capabilities := search.Capabilities()
		if !capabilities.AllowedDomains || !capabilities.ExcludedDomains || !capabilities.MaxResults {
			t.Fatalf("incomplete search capabilities for %q: %#v", provider, capabilities)
		}
	}
	if search := catalog.SearchProvider("anthropic", config.Resolved{Values: map[string]config.Value{
		"model": {Value: "fixture-model", Source: "test"}, "base_url": {Value: "https://custom.example", Source: "test"},
	}}, lookup); search != nil {
		t.Fatal("Anthropic custom endpoint received the hosted search credential path")
	}
	if search := catalog.SearchProvider("openai", cfg, lookup); search != nil {
		t.Fatal("OpenAI API key provider was mistaken for Codex subscription sign-in")
	}
}

func TestBraveSearchEnvironmentCredentialUsesAdapterName(t *testing.T) {
	key, err := core.EnvironmentAPIKey(context.Background(), "brave-search", func(name string) string {
		if name == "BRAVE_SEARCH_API_KEY" {
			return "brave-test-key"
		}
		return ""
	})
	if err != nil || key != "brave-test-key" {
		t.Fatalf("resolved Brave credential = %q, %v", key, err)
	}
}
