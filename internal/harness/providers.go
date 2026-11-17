package harness

import (
	"context"
	"fmt"

	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/provideradapters"
	"github.com/diasYuri/opera/internal/providercatalog"
	"github.com/diasYuri/y/pkg/providers"
)

// ModelInfo is the UI-safe subset of a provider catalog needed for selection.
type ModelInfo struct {
	ID        string
	Reasoning bool
}

func nativeCatalog() (*providercatalog.Catalog, error) { return provideradapters.DefaultCatalog() }

func configuredProvider(ctx context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error), catalogs ...*providercatalog.Catalog) (providers.Provider, error) {
	var catalog *providercatalog.Catalog
	if len(catalogs) > 0 {
		catalog = catalogs[0]
	}
	if catalog == nil {
		var err error
		catalog, err = nativeCatalog()
		if err != nil {
			return nil, err
		}
	}
	return catalog.Create(ctx, cfg, lookup)
}

func providerFactory(ctx context.Context, cfg config.Resolved) (providers.Provider, error) {
	return configuredProvider(ctx, cfg, nil)
}

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
	out := make([]entry, 0, len(models))
	for _, model := range models {
		out = append(out, entry{model, provider.Capabilities(model.ID)})
	}
	return out, nil
}

func ModelChoices(ctx context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error), providers ...*providercatalog.Catalog) ([]string, error) {
	catalog, err := ModelCatalog(ctx, cfg, lookup, providers...)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(catalog))
	for _, model := range catalog {
		out = append(out, fmt.Sprintf("%s %s", cfg.String("provider"), model.ID))
	}
	return out, nil
}

func ModelCatalog(ctx context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error), catalogs ...*providercatalog.Catalog) ([]ModelInfo, error) {
	provider, err := configuredProvider(ctx, cfg, lookup, catalogs...)
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
