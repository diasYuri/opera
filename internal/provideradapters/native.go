// Package provideradapters contains the native provider implementations used by
// Opera's default composition. Applications can supply a different catalog.
package provideradapters

import (
	"context"
	"errors"

	"github.com/diasYuri/opera/internal/chatgpt"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/providercatalog"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/anthropic"
	"github.com/diasYuri/y/pkg/providers/auth"
	"github.com/diasYuri/y/pkg/providers/google"
	"github.com/diasYuri/y/pkg/providers/openai"
)

func DefaultCatalog() (*providercatalog.Catalog, error) {
	noEnvironment := func(string) string { return "" }
	return providercatalog.New(
		providercatalog.Entry{ID: "openai-subscription", Name: "openai", Description: "Sign in with /login · ChatGPT subscription models", Factory: func(_ context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error)) (providers.Provider, error) {
			if cfg.String("base_url") != "" {
				return nil, errors.New("openai-subscription uses the public OpenAI API and does not accept base_url")
			}
			return chatgpt.NewProvider(lookup), nil
		}},
		providercatalog.Entry{ID: "openai", Name: "OpenAI API", Description: "Requires an OpenAI API credential", Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
			opts := []openai.Option{openai.WithEnvLookup(noEnvironment)}
			if base := cfg.String("base_url"); base != "" {
				opts = append(opts, openai.WithBaseURL(base))
			}
			return openai.New(opts...), nil
		}},
		providercatalog.Entry{ID: "anthropic", Name: "Anthropic API", Description: "Requires an Anthropic API credential", Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
			opts := []anthropic.Option{anthropic.WithEnvLookup(noEnvironment)}
			if base := cfg.String("base_url"); base != "" {
				opts = append(opts, anthropic.WithBaseURL(base))
			}
			return anthropic.New(opts...), nil
		}},
		providercatalog.Entry{ID: "google", Name: "Google API", Description: "Requires a Google API credential", Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
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
	)
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
