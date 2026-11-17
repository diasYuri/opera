package deepseek

import (
	"github.com/diasYuri/opera/internal/adapters/providers/openai_compatible"
	"github.com/diasYuri/opera/internal/providercatalog"
)

func Entry() providercatalog.Entry {
	return openai_compatible.NamedEntry("deepseek", "DeepSeek", "https://api.deepseek.com", &providercatalog.LoginDescriptor{ID: "deepseek", Name: "DeepSeek", Description: "DeepSeek API key", Methods: []providercatalog.LoginMethod{{ID: "api-key", Name: "DeepSeek API key", Description: "Use a DeepSeek API key", Kind: "api-key", ProviderID: "deepseek"}}})
}
