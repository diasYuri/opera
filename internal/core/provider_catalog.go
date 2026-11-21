// Package core composes the runtime and default provider catalog.
package core

import (
	"context"
	"fmt"

	"github.com/diasYuri/opera/internal/adapters/providers/anthropic"
	"github.com/diasYuri/opera/internal/adapters/providers/custom"
	"github.com/diasYuri/opera/internal/adapters/providers/deepseek"
	"github.com/diasYuri/opera/internal/adapters/providers/google"
	"github.com/diasYuri/opera/internal/adapters/providers/groq"
	"github.com/diasYuri/opera/internal/adapters/providers/openai"
	"github.com/diasYuri/opera/internal/adapters/providers/openai/subscription"
	"github.com/diasYuri/opera/internal/adapters/providers/openai_compatible"
	"github.com/diasYuri/opera/internal/adapters/providers/openrouter"
	bravesearch "github.com/diasYuri/opera/internal/adapters/search/brave"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/providers/auth"
)

func DefaultCatalog(customProviders ...config.CustomProvider) (*providercatalog.Catalog, error) {
	entries := append([]providercatalog.Entry(nil), openai.Entries()...)
	entries = append(entries, subscription.Entry(), anthropic.Entry(), google.Entry(), openai_compatible.Entry(), openrouter.Entry(), groq.Entry(), deepseek.Entry())
	for _, provider := range customProviders {
		if err := provider.Validate(); err != nil {
			return nil, fmt.Errorf("custom provider %q: %w", provider.Name, err)
		}
		entries = append(entries, custom.Entry(provider))
	}
	return providercatalog.New(entries...)
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

// EnvironmentAPIKey resolves standard SDK environment variables and provider-owned
// conventions through the composed catalog.
func EnvironmentAPIKey(ctx context.Context, provider string, lookup func(string) string) (string, error) {
	if provider == "brave-search" {
		return bravesearch.EnvironmentCredential(lookup), nil
	}
	catalog, err := DefaultCatalog()
	if err != nil {
		return "", err
	}
	if value := catalog.EnvironmentCredential(provider, lookup); value != "" {
		return value, nil
	}
	source := auth.NewEnvSource().WithLookup(lookup)
	return source.Resolve(ctx, provider)
}

func CustomAPIKey(ctx context.Context, provider config.CustomProvider) (string, error) {
	return custom.CustomAPIKey(ctx, provider)
}
