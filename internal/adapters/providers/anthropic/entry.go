package anthropic

import (
	"context"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/providers"
	yanthropic "github.com/diasYuri/y/pkg/providers/anthropic"
	"github.com/diasYuri/y/pkg/search"
)

func Entry() providercatalog.Entry {
	login := &providercatalog.LoginDescriptor{ID: "anthropic", Name: "Anthropic", Description: "Claude account or Anthropic API key", Methods: []providercatalog.LoginMethod{
		{ID: "claude", Name: "Sign in with Claude", Description: "Claude Pro/Max OAuth", Kind: "anthropic-oauth", ProviderID: "anthropic", Authenticator: authenticator{}},
		{ID: "api-key", Name: "Anthropic API key", Description: "Use an Anthropic API key", Kind: "api-key", ProviderID: "anthropic"},
	}}
	return providercatalog.Entry{ID: "anthropic", Name: "Anthropic API", Description: "Requires Anthropic credentials", Login: login, EnvironmentCredential: EnvironmentCredential, SearchFactory: func(cfg config.Resolved, lookup func(context.Context, string) (string, error)) search.Provider {
		if cfg.String("base_url") != "" {
			return nil
		}
		return NewSearchProvider(cfg.String("model"), lookup)
	}, Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
		opts := []yanthropic.Option{yanthropic.WithEnvLookup(func(string) string { return "" })}
		if base := cfg.String("base_url"); base != "" {
			opts = append(opts, yanthropic.WithBaseURL(base))
		}
		return yanthropic.New(opts...), nil
	}}
}
