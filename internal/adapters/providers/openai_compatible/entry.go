package openai_compatible

import (
	"context"
	"errors"

	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/providercatalog"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/openai"
)

func Entry() providercatalog.Entry {
	return providercatalog.Entry{ID: "openai-compatible", Name: "OpenAI-compatible endpoint", Description: "Requires a configured base URL and API credential", Factory: func(_ context.Context, cfg config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
		if cfg.String("base_url") == "" {
			return nil, errors.New("openai-compatible requires an explicit user-controlled base_url")
		}
		return openai.NewCompatible(cfg.String("base_url"), openai.WithEnvLookup(func(string) string { return "" })), nil
	}}
}
