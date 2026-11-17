package google

import (
	"context"

	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/providercatalog"
	"github.com/diasYuri/y/pkg/providers"
	ygoogle "github.com/diasYuri/y/pkg/providers/google"
)

func Entry() providercatalog.Entry {
	login := &providercatalog.LoginDescriptor{ID: "google", Name: "Google Gemini", Description: "Gemini API key", Methods: []providercatalog.LoginMethod{{ID: "api-key", Name: "Gemini API key", Description: "Use a Gemini API key", Kind: "api-key", ProviderID: "google"}}}
	return providercatalog.Entry{ID: "google", Name: "Google API", Description: "Requires a Gemini API credential", Login: login, Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
		opts := []ygoogle.Option{ygoogle.WithEnvLookup(func(string) string { return "" })}
		if base := cfg.String("base_url"); base != "" {
			opts = append(opts, ygoogle.WithBaseURL(base))
		}
		return ygoogle.New(opts...), nil
	}}
}
