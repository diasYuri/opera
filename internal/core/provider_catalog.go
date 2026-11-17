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

// SubscriptionRecord and its factory keep CLI authentication flows behind core's
// provider composition boundary while preserving the provider-owned protocol.
type SubscriptionRecord = subscription.Record

const SubscriptionProviderID = subscription.ProviderID

func NewSubscriptionClient() *subscription.Client { return subscription.NewClient() }

func NewSubscriptionProvider(lookup func(context.Context, string) (string, error)) *subscription.Provider {
	return subscription.NewProvider(lookup)
}

func CustomAPIKey(ctx context.Context, provider config.CustomProvider) (string, error) {
	return custom.CustomAPIKey(ctx, provider)
}
