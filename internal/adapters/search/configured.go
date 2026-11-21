package search

import (
	"context"

	searchcontract "github.com/diasYuri/y/pkg/search"
)

type configuredProvider struct {
	provider searchcontract.Provider
	defaults searchcontract.Request
}

// WithDefaults applies user-configured defaults only when a search request
// leaves a supported control unset. Explicit request controls take precedence.
func WithDefaults(provider searchcontract.Provider, defaults searchcontract.Request) searchcontract.Provider {
	if provider == nil {
		return nil
	}
	defaults.AllowedDomains = append([]string(nil), defaults.AllowedDomains...)
	defaults.ExcludedDomains = append([]string(nil), defaults.ExcludedDomains...)
	return configuredProvider{provider: provider, defaults: defaults}
}

func (p configuredProvider) Capabilities() searchcontract.Capabilities {
	return p.provider.Capabilities()
}

func (p configuredProvider) Search(ctx context.Context, request searchcontract.Request) (searchcontract.Response, error) {
	if len(request.AllowedDomains) == 0 {
		request.AllowedDomains = append([]string(nil), p.defaults.AllowedDomains...)
	}
	if len(request.ExcludedDomains) == 0 {
		request.ExcludedDomains = append([]string(nil), p.defaults.ExcludedDomains...)
	}
	if request.ContextSize == "" {
		request.ContextSize = p.defaults.ContextSize
	}
	if request.MaxResults == 0 {
		request.MaxResults = p.defaults.MaxResults
	}
	if request.Country == "" {
		request.Country = p.defaults.Country
	}
	if request.Language == "" {
		request.Language = p.defaults.Language
	}
	if request.SafeSearch == "" {
		request.SafeSearch = p.defaults.SafeSearch
	}
	return p.provider.Search(ctx, request)
}
