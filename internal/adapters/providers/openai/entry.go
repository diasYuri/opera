// Package openai contributes the OpenAI API provider and ChatGPT subscription provider.
package openai

import (
	"context"

	"github.com/diasYuri/opera/internal/adapters/providers/openai/subscription"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/providers"
	yopenai "github.com/diasYuri/y/pkg/providers/openai"
)

func Entries() []providercatalog.Entry {
	noEnvironment := func(string) string { return "" }
	login := &providercatalog.LoginDescriptor{
		ID: "openai", Name: "OpenAI", Description: "OpenAI API key or ChatGPT subscription",
		Methods: []providercatalog.LoginMethod{
			{ID: "chatgpt", Name: "Sign in with ChatGPT", Description: "ChatGPT subscription OAuth", Kind: "chatgpt-oauth", ProviderID: subscription.ProviderID, Authenticator: subscription.Authenticator()},
			{ID: "api-key", Name: "OpenAI API key", Description: "Use an OpenAI API key", Kind: "api-key", ProviderID: "openai"},
		},
	}
	return []providercatalog.Entry{
		{ID: "openai", Name: "OpenAI API", Description: "Requires an OpenAI API credential", Login: login, KnownModels: yopenai.CuratedModels, Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
			opts := []yopenai.Option{yopenai.WithEnvLookup(noEnvironment)}
			if base := cfg.String("base_url"); base != "" {
				opts = append(opts, yopenai.WithBaseURL(base))
			}
			return yopenai.New(opts...), nil
		}},
	}
}
