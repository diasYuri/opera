package subscription

import (
	"context"
	"errors"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/providers"
)

func Entry() providercatalog.Entry {
	return providercatalog.Entry{ID: ProviderID, Name: "openai", Description: "Sign in with /login · ChatGPT subscription models", Factory: func(_ context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error)) (providers.Provider, error) {
		if cfg.String("base_url") != "" {
			return nil, errors.New("openai-subscription uses the public OpenAI API and does not accept base_url")
		}
		return NewProvider(lookup), nil
	}}
}
