package openrouter

import (
	"github.com/diasYuri/opera/internal/adapters/providers/openai_compatible"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
)

func Entry() providercatalog.Entry {
	return openai_compatible.NamedEntry("openrouter", "OpenRouter", "https://openrouter.ai/api/v1", &providercatalog.LoginDescriptor{ID: "openrouter", Name: "OpenRouter", Description: "OpenRouter account or API key", Methods: []providercatalog.LoginMethod{
		{ID: "oauth", Name: "Sign in with OpenRouter", Description: "OpenRouter OAuth", Kind: "openrouter-oauth", ProviderID: "openrouter"},
		{ID: "api-key", Name: "OpenRouter API key", Description: "Use an OpenRouter API key", Kind: "api-key", ProviderID: "openrouter"},
	}})
}
