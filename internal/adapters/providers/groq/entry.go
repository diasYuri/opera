package groq

import (
	"github.com/diasYuri/opera/internal/adapters/providers/openai_compatible"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
)

func Entry() providercatalog.Entry {
	return openai_compatible.NamedEntry("groq", "Groq", "https://api.groq.com/openai/v1", &providercatalog.LoginDescriptor{ID: "groq", Name: "Groq", Description: "Groq API key", Methods: []providercatalog.LoginMethod{{ID: "api-key", Name: "Groq API key", Description: "Use a Groq API key", Kind: "api-key", ProviderID: "groq"}}})
}
