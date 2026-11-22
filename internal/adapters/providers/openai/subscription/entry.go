package subscription

import (
	"context"
	"errors"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/search"
)

func Entry() providercatalog.Entry {
	return providercatalog.Entry{ID: ProviderID, Name: "openai", Description: "Sign in with /login · ChatGPT subscription models", FixedEndpoint: true, SearchName: "OpenAI · Codex", SearchDescription: "Hosted web search using the signed-in ChatGPT account", SearchFactory: func(_ config.Resolved, lookup func(context.Context, string) (string, error)) search.Provider {
		return NewSearchProvider(lookup)
	}, Factory: func(_ context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error)) (providers.Provider, error) {
		if cfg.String("base_url") != "" {
			return nil, errors.New("openai-subscription uses the public OpenAI API and does not accept base_url")
		}
		return NewProvider(lookup), nil
	}}
}
