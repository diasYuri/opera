// Package provideradapters composes the default provider modules into one catalog.
package provideradapters

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
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/providercatalog"
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
