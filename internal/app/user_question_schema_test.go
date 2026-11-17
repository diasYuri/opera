package app

import (
	"context"
	"encoding/json"
	"github.com/diasYuri/y/pkg/tools"
	"testing"
)

func TestQuestionBrokerPublishesValidJSONSchema(t *testing.T) {
	registry := tools.NewRegistry()
	t.Cleanup(func() {
		if err := registry.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := NewQuestionBroker().Register(registry); err != nil {
		t.Fatal(err)
	}
	descriptor, _, ok := registry.Get("user_question")
	if !ok || !json.Valid(descriptor.InputSchema) {
		t.Fatal("user_question must publish a valid JSON schema")
	}
}
