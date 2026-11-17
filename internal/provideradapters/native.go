// Package provideradapters contains the native provider implementations used by
// Opera's default composition. Applications can supply a different catalog.
package provideradapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/diasYuri/opera/internal/chatgpt"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/providercatalog"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/anthropic"
	"github.com/diasYuri/y/pkg/providers/auth"
	"github.com/diasYuri/y/pkg/providers/google"
	"github.com/diasYuri/y/pkg/providers/openai"
)

func DefaultCatalog(customProviders ...config.CustomProvider) (*providercatalog.Catalog, error) {
	noEnvironment := func(string) string { return "" }
	openaiLogin := &providercatalog.LoginDescriptor{
		ID: "openai", Name: "OpenAI", Description: "OpenAI API key or ChatGPT subscription",
		Methods: []providercatalog.LoginMethod{
			{ID: "chatgpt", Name: "Sign in with ChatGPT", Description: "ChatGPT subscription OAuth", Kind: "chatgpt-oauth", ProviderID: "openai-subscription"},
			{ID: "api-key", Name: "OpenAI API key", Description: "Use an OpenAI API key", Kind: "api-key", ProviderID: "openai"},
		},
	}
	entries := []providercatalog.Entry{
		providercatalog.Entry{ID: "openai-subscription", Name: "openai", Description: "Sign in with /login · ChatGPT subscription models", Factory: func(_ context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error)) (providers.Provider, error) {
			if cfg.String("base_url") != "" {
				return nil, errors.New("openai-subscription uses the public OpenAI API and does not accept base_url")
			}
			return chatgpt.NewProvider(lookup), nil
		}},
		providercatalog.Entry{ID: "openai", Name: "OpenAI API", Description: "Requires an OpenAI API credential", Login: openaiLogin, Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
			opts := []openai.Option{openai.WithEnvLookup(noEnvironment)}
			if base := cfg.String("base_url"); base != "" {
				opts = append(opts, openai.WithBaseURL(base))
			}
			return openai.New(opts...), nil
		}},
		providercatalog.Entry{ID: "anthropic", Name: "Anthropic API", Description: "Requires Anthropic credentials", Login: &providercatalog.LoginDescriptor{
			ID: "anthropic", Name: "Anthropic", Description: "Claude account or Anthropic API key",
			Methods: []providercatalog.LoginMethod{
				{ID: "claude", Name: "Sign in with Claude", Description: "Claude Pro/Max OAuth", Kind: "anthropic-oauth", ProviderID: "anthropic"},
				{ID: "api-key", Name: "Anthropic API key", Description: "Use an Anthropic API key", Kind: "api-key", ProviderID: "anthropic"},
			},
		}, Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
			opts := []anthropic.Option{anthropic.WithEnvLookup(noEnvironment)}
			if base := cfg.String("base_url"); base != "" {
				opts = append(opts, anthropic.WithBaseURL(base))
			}
			return anthropic.New(opts...), nil
		}},
		providercatalog.Entry{ID: "google", Name: "Google API", Description: "Requires a Gemini API credential", Login: &providercatalog.LoginDescriptor{
			ID: "google", Name: "Google Gemini", Description: "Gemini API key",
			Methods: []providercatalog.LoginMethod{{ID: "api-key", Name: "Gemini API key", Description: "Use a Gemini API key", Kind: "api-key", ProviderID: "google"}},
		}, Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
			opts := []google.Option{google.WithEnvLookup(noEnvironment)}
			if base := cfg.String("base_url"); base != "" {
				opts = append(opts, google.WithBaseURL(base))
			}
			return google.New(opts...), nil
		}},
		providercatalog.Entry{ID: "openai-compatible", Name: "OpenAI-compatible endpoint", Description: "Requires a configured base URL and API credential", Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
			if cfg.String("base_url") == "" {
				return nil, errors.New("openai-compatible requires an explicit user-controlled base_url")
			}
			return openai.NewCompatible(cfg.String("base_url"), openai.WithEnvLookup(noEnvironment)), nil
		}},
		compatibleEntry("openrouter", "OpenRouter", "https://openrouter.ai/api/v1", &providercatalog.LoginDescriptor{
			ID: "openrouter", Name: "OpenRouter", Description: "OpenRouter account or API key",
			Methods: []providercatalog.LoginMethod{
				{ID: "oauth", Name: "Sign in with OpenRouter", Description: "OpenRouter OAuth", Kind: "openrouter-oauth", ProviderID: "openrouter"},
				{ID: "api-key", Name: "OpenRouter API key", Description: "Use an OpenRouter API key", Kind: "api-key", ProviderID: "openrouter"},
			},
		}),
		compatibleEntry("groq", "Groq", "https://api.groq.com/openai/v1", &providercatalog.LoginDescriptor{
			ID: "groq", Name: "Groq", Description: "Groq API key",
			Methods: []providercatalog.LoginMethod{{ID: "api-key", Name: "Groq API key", Description: "Use a Groq API key", Kind: "api-key", ProviderID: "groq"}},
		}),
		compatibleEntry("deepseek", "DeepSeek", "https://api.deepseek.com", &providercatalog.LoginDescriptor{
			ID: "deepseek", Name: "DeepSeek", Description: "DeepSeek API key",
			Methods: []providercatalog.LoginMethod{{ID: "api-key", Name: "DeepSeek API key", Description: "Use a DeepSeek API key", Kind: "api-key", ProviderID: "deepseek"}},
		}),
	}
	for _, provider := range customProviders {
		if err := provider.Validate(); err != nil {
			return nil, fmt.Errorf("custom provider %q: %w", provider.Name, err)
		}
		entries = append(entries, customEntry(provider))
	}
	return providercatalog.New(entries...)
}

func compatibleEntry(id, name, endpoint string, login *providercatalog.LoginDescriptor) providercatalog.Entry {
	return providercatalog.Entry{ID: id, Name: name, Description: "OpenAI-compatible Chat Completions API", Login: login, Factory: func(_ context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error)) (providers.Provider, error) {
		baseURL := cfg.String("base_url")
		if baseURL == "" {
			baseURL = endpoint
		}
		provider := openai.NewCompatible(baseURL, openai.WithEnvLookup(func(string) string { return "" }))
		return compatibleProvider{Provider: provider, id: id, baseURL: baseURL, lookup: lookup}, nil
	}}
}

// CredentialResolver adapts Opera's explicit credential lookup to Y's public
// auth resolver contract.
type CredentialResolver struct {
	Lookup func(context.Context, string) (string, error)
}

func (r CredentialResolver) ResolveAPIKey(ctx context.Context, request auth.ResolveRequest) (string, error) {
	if r.Lookup == nil {
		return "", nil
	}
	return r.Lookup(ctx, request.ProviderID)
}
