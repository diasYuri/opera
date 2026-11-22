package core

import (
	"context"
	"errors"
	"fmt"

	searchadapter "github.com/diasYuri/opera/internal/adapters/search"
	bravesearch "github.com/diasYuri/opera/internal/adapters/search/brave"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/search"
)

type searchSelection struct {
	id       string
	provider string
	model    string
}

func selectSearch(cfg config.Resolved, opts Options) (searchSelection, error) {
	if opts.SearchProvider != nil {
		return searchSelection{id: "provided"}, nil
	}
	selection, provider := cfg.String("search.provider"), cfg.String("provider")
	switch selection {
	case "":
		// A project cannot activate outbound search by selecting a model provider.
		if cfg.Values["provider"].Source == "project" && cfg.Values["search.provider"].Source == "default" {
			break
		}
		fallthrough
	case "native":
		if opts.ProviderCatalog.HasNativeSearch(provider, cfg) {
			return searchSelection{id: "native", provider: provider, model: cfg.String("model")}, nil
		}
		if selection == "native" {
			return searchSelection{}, fmt.Errorf("native web search is not available for provider %q", provider)
		}
	case "disabled":
	case "brave":
		return searchSelection{id: "brave", provider: "brave"}, nil
	default:
		return searchSelection{}, fmt.Errorf("unsupported search provider %q", selection)
	}
	return searchSelection{id: "disabled"}, nil
}

func (s searchSelection) checkInputs(opts Options) error {
	if s.id == "brave" && opts.Credential == nil {
		return errors.New("search provider Brave Search is configured but no credential resolver is available")
	}
	return nil
}

func (s searchSelection) create(cfg config.Resolved, opts Options) (search.Provider, error) {
	var provider search.Provider
	switch s.id {
	case "provided":
		return opts.SearchProvider, nil
	case "native":
		if opts.Management {
			cfg = cloneConfig(cfg)
			cfg.Values["provider"] = config.Value{Value: s.provider, Source: "management"}
			cfg.Values["model"] = config.Value{Value: s.model, Source: "management"}
		}
		provider = opts.ProviderCatalog.SearchProvider(s.provider, cfg, opts.Credential)
		if provider == nil {
			return nil, errors.New("native web search factory returned nil")
		}
	case "brave":
		provider = bravesearch.NewWithCredentialResolver(func(ctx context.Context) (string, error) {
			key, err := opts.Credential(ctx, "brave-search")
			if err != nil {
				return "", fmt.Errorf("resolve Brave Search credential: %w", err)
			}
			return key, nil
		})
	default:
		return nil, nil
	}
	defaults := cfg.WebSearchDefaults(s.provider)
	return searchadapter.WithDefaults(provider, search.Request{
		AllowedDomains: defaults.AllowedDomains, ExcludedDomains: defaults.ExcludedDomains,
		ContextSize: search.ContextSize(defaults.ContextSize), MaxResults: defaults.MaxResults,
		Country: defaults.Country, Language: defaults.Language, SafeSearch: search.SafeSearch(defaults.SafeSearch),
	}), nil
}

func webSearchEntries(catalog *providercatalog.Catalog) []providercatalog.SearchEntry {
	return append(catalog.SearchEntries(), providercatalog.SearchEntry{
		ID: "brave", Name: "Brave Search", Description: "Brave Search API; requires a Brave Search API key",
		AuthMethods: bravesearch.AuthenticationMethods(),
	})
}

func WebSearchChoices(cfg config.Resolved, catalog *providercatalog.Catalog) (choices []providercatalog.SearchEntry, current string) {
	choices = webSearchEntries(catalog)
	if selected, err := selectSearch(cfg, Options{ProviderCatalog: catalog}); err == nil {
		current = selected.provider
	}
	return choices, current
}
