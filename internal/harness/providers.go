package harness

import (
	"context"
	"errors"

	"github.com/diasYuri/opera/internal/chatgpt"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/anthropic"
	"github.com/diasYuri/y/pkg/providers/auth"
	"github.com/diasYuri/y/pkg/providers/google"
	"github.com/diasYuri/y/pkg/providers/openai"
)

type credentialResolver struct {
	lookup func(context.Context, string) (string, error)
}

// ModelInfo is the UI-safe subset of a provider catalog needed for selection.
type ModelInfo struct {
	ID        string
	Reasoning bool
}

func (r credentialResolver) ResolveAPIKey(ctx context.Context, request auth.ResolveRequest) (string, error) {
	return r.lookup(ctx, request.ProviderID)
}

func providerFactory(ctx context.Context, cfg config.Resolved) (providers.Provider, error) {
	return configuredProvider(ctx, cfg, nil)
}

func configuredProvider(_ context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error)) (providers.Provider, error) {
	noEnvironment := func(string) string { return "" }
	switch cfg.String("provider") {
	case chatgpt.ProviderID:
		if cfg.String("base_url") != "" {
			return nil, errors.New("openai-subscription uses the public OpenAI API and does not accept base_url")
		}
		return chatgpt.NewProvider(lookup), nil
	case "openai":
		opts := []openai.Option{openai.WithEnvLookup(noEnvironment)}
		if base := cfg.String("base_url"); base != "" {
			opts = append(opts, openai.WithBaseURL(base))
		}
		return openai.New(opts...), nil
	case "anthropic":
		opts := []anthropic.Option{anthropic.WithEnvLookup(noEnvironment)}
		if base := cfg.String("base_url"); base != "" {
			opts = append(opts, anthropic.WithBaseURL(base))
		}
		return anthropic.New(opts...), nil
	case "google":
		opts := []google.Option{google.WithEnvLookup(noEnvironment)}
		if base := cfg.String("base_url"); base != "" {
			opts = append(opts, google.WithBaseURL(base))
		}
		return google.New(opts...), nil
	case "openai-compatible":
		if cfg.String("base_url") == "" {
			return nil, errors.New("openai-compatible requires an explicit user-controlled base_url")
		}
		return openai.NewCompatible(cfg.String("base_url"), openai.WithEnvLookup(noEnvironment)), nil
	}
	return nil, errors.New("provider is not supported")
}

// Models reports model metadata and capabilities. Subscription catalogs require
// credentials for the selected ChatGPT account.
func Models(ctx context.Context, cfg config.Resolved, credentials ...func(context.Context, string) (string, error)) (any, error) {
	var lookup func(context.Context, string) (string, error)
	if len(credentials) > 0 {
		lookup = credentials[0]
	}
	provider, err := configuredProvider(ctx, cfg, lookup)
	if err != nil {
		return nil, err
	}
	defer func() { _ = provider.Close() }()
	models, err := provider.Models(ctx)
	if err != nil {
		return nil, err
	}
	type entry struct {
		Model        any                    `json:"model"`
		Capabilities providers.Capabilities `json:"capabilities"`
	}
	out := []entry{}
	for _, model := range models {
		out = append(out, entry{model, provider.Capabilities(model.ID)})
	}
	return out, nil
}

// ModelChoices uses the same account-aware catalog for CLI and interactive chat.
func ModelChoices(ctx context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error)) ([]string, error) {
	catalog, err := ModelCatalog(ctx, cfg, lookup)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(catalog))
	for _, model := range catalog {
		out = append(out, cfg.String("provider")+" "+model.ID)
	}
	return out, nil
}

func ModelCatalog(ctx context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error)) ([]ModelInfo, error) {
	provider, err := configuredProvider(ctx, cfg, lookup)
	if err != nil {
		return nil, err
	}
	defer func() { _ = provider.Close() }()
	models, err := provider.Models(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(models))
	for _, model := range models {
		out = append(out, ModelInfo{ID: model.ID, Reasoning: provider.Capabilities(model.ID).Reasoning})
	}
	return out, nil
}
