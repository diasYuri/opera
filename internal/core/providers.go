package core

import (
	"context"
	"fmt"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/providers"
)

// ModelInfo is the UI-safe subset of a provider catalog needed for selection.
type ModelInfo struct {
	ID        string
	Reasoning bool
}

func nativeCatalog(cfg config.Resolved) (*providercatalog.Catalog, error) {
	return DefaultCatalog(cfg.CustomProviders...)
}

func configuredProvider(ctx context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error), catalogs ...*providercatalog.Catalog) (providers.Provider, error) {
	var catalog *providercatalog.Catalog
	if len(catalogs) > 0 {
		catalog = catalogs[0]
	}
	if catalog == nil {
		var err error
		catalog, err = nativeCatalog(cfg)
		if err != nil {
			return nil, err
		}
	}
	return catalog.Create(ctx, cfg, lookup)
}

func Models(ctx context.Context, cfg config.Resolved, credentials ...func(context.Context, string) (string, error)) (any, error) {
	var lookup func(context.Context, string) (string, error)
	if len(credentials) > 0 {
		lookup = credentials[0]
	}
	return ModelsWithCatalog(ctx, cfg, lookup, nil)
}

// ModelsWithCatalog lists models from the same catalog used for execution.
func ModelsWithCatalog(ctx context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error), catalog *providercatalog.Catalog) (any, error) {
	provider, err := configuredProvider(ctx, cfg, lookup, catalog)
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
