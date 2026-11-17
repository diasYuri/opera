package core_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/openai"
)

type serializationRoundTripper func(*http.Request) (*http.Response, error)

func (f serializationRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func strictCompatibleSchema(schema any) bool {
	value, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	switch value["type"] {
	case "object":
		properties, ok := value["properties"].(map[string]any)
		if !ok || value["additionalProperties"] != false {
			return false
		}
		requiredValues, ok := value["required"].([]any)
		if !ok || len(requiredValues) != len(properties) {
			return false
		}
		required := make(map[string]bool, len(requiredValues))
		for _, name := range requiredValues {
			key, ok := name.(string)
			if !ok {
				return false
			}
			required[key] = true
		}
		for name, property := range properties {
			if !required[name] || !strictCompatibleSchema(property) {
				return false
			}
		}
	case "array":
		return strictCompatibleSchema(value["items"])
	}
	return true
}

func TestOpenAIRequestSerializesOperaToolContext(t *testing.T) {
	cfg := configuration(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		var questionToolFound bool
		for _, rawTool := range body["tools"].([]any) {
			tool, ok := rawTool.(map[string]any)
			if !ok || tool["name"] != "user_question" {
				continue
			}
			questionToolFound = true
			if !strictCompatibleSchema(tool["parameters"]) {
				t.Error("user_question schema is not compatible with strict function parameters")
			}
		}
		if !questionToolFound {
			t.Error("user_question tool was not registered")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture\",\"status\":\"completed\"}}\n\n")
	}))
	defer server.Close()

	provider := openai.New(
		openai.WithAPIKey("fixture-key"),
		openai.WithBaseURL(server.URL),
		openai.WithHTTPClient(&http.Client{Transport: serializationRoundTripper(func(r *http.Request) (*http.Response, error) {
			return http.DefaultTransport.RoundTrip(r)
		})}),
	)
	runtime, err := core.Open(context.Background(), cfg, core.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, runtime)

	result, err := runtime.Service.Execute(context.Background(), app.Request{Prompt: "Explain this project"}, nil)
	if err != nil || result.Text != "ok" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if requests.Load() != 1 {
		t.Fatalf("got %d provider requests, want 1", requests.Load())
	}
}
