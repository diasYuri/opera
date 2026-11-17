package groq

import (
	"github.com/diasYuri/opera/internal/adapters/providers/openai_compatible"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
)

func EnvironmentCredential(lookup func(string) string) string { return lookup("GROQ_API_KEY") }

func Entry() providercatalog.Entry {
	entry := openai_compatible.NamedEntry("groq", "Groq", "https://api.groq.com/openai/v1", &providercatalog.LoginDescriptor{ID: "groq", Name: "Groq", Description: "Groq API key", Methods: []providercatalog.LoginMethod{{ID: "api-key", Name: "Groq API key", Description: "Use a Groq API key", Kind: "api-key", ProviderID: "groq"}}})
	entry.EnvironmentCredential = EnvironmentCredential
	return entry
}
