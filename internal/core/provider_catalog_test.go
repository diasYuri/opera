package core_test

import (
	"testing"

	"github.com/diasYuri/opera/internal/core"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
)

func TestDefaultCatalogRoutesChatGPTLoginByTargetProvider(t *testing.T) {
	catalog, err := core.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}

	var openAI *providercatalog.LoginDescriptor
	for _, item := range catalog.Logins() {
		descriptor := item
		if descriptor.ID == "openai" {
			openAI = &descriptor
		}
		if descriptor.ID == "openai-subscription" {
			t.Fatal("subscription login duplicated the OpenAI login group")
		}
	}
	if openAI == nil || len(openAI.Methods) != 2 {
		t.Fatalf("OpenAI login descriptor = %#v", openAI)
	}
	var chatGPTMethod bool
	for _, method := range openAI.Methods {
		if method.ID == "chatgpt" {
			chatGPTMethod = method.ProviderID == "openai-subscription" && method.Authenticator != nil
		}
	}
	if !chatGPTMethod {
		t.Fatal("ChatGPT method is missing its subscription authenticator")
	}
	if catalog.Authenticator("openai-subscription") == nil {
		t.Fatal("subscription authenticator was not indexed by target provider")
	}
	if catalog.Authenticator("openai") != nil {
		t.Fatal("OpenAI API login unexpectedly resolved to the subscription authenticator")
	}
}
