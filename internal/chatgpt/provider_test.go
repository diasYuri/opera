package chatgpt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestAccountCatalogPreservesVisibilityAndOrder(t *testing.T) {
	p := NewProvider(func(_ context.Context, id string) (string, error) {
		if id != ProviderID {
			t.Error(id)
		}
		return "subscription-token", nil
	})
	p.http.Transport = routeTransport{roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer subscription-token" || r.URL.Path != "/v1/models" {
			t.Error("wrong catalog authentication")
		}
		return fixtureResponse(`{"models":[{"slug":"second","display_name":"Second","visibility":"list"},{"slug":"hidden","visibility":"hide"},{"slug":"first","display_name":"First","visibility":"list"}]}`), nil
	})}
	models, err := p.Models(context.Background())
	if err != nil || len(models) != 2 || models[0].ID != "second" || models[1].ID != "first" || models[0].Name != "Second" {
		t.Fatal(models, err)
	}
	p.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return fixtureResponse(`{"data":[{"id":"api-only"}]}`), nil
	})
	if _, err := p.Models(context.Background()); err == nil {
		t.Fatal("API-key catalog accepted for subscription")
	}
}

func TestSubscriptionStreamingContractAndTerminalEvents(t *testing.T) {
	for _, test := range []struct{ name, terminal, wantError string }{
		{"completed", `{"type":"response.completed","response":{"id":"response-1","status":"completed"}}`, ""},
		{"truncated", "", "interrupted"},
		{"incomplete", `{"type":"response.incomplete","response":{"status":"incomplete"}}`, "incomplete"},
		{"quota", `{"type":"response.failed","response":{"error":{"code":"subscription_sharing_usage_limit_exceeded"}}}`, "https://chatgpt.com/settings/usage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := NewProvider(nil)
			p.http.Transport = routeTransport{roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Authorization") != "Bearer subscription-token" || r.URL.String() != resource+"/responses" {
					t.Error("wrong inference route")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["store"] != false || body["stream"] != true {
					t.Error("subscription requires store=false stream=true")
				}
				for _, key := range []string{"max_output_tokens", "temperature", "previous_response_id", "prompt_cache_retention"} {
					if _, ok := body[key]; ok {
						t.Error("unsupported field", key)
					}
				}
				for _, item := range body["input"].([]any) {
					if item.(map[string]any)["role"] == "system" {
						t.Error("unsupported system input")
					}
				}
				tools := body["tools"].([]any)
				if tools[0].(map[string]any)["type"] != "namespace" {
					t.Error("tools not namespaced")
				}
				bodyText := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"
				if test.terminal != "" {
					bodyText += "data: " + test.terminal + "\n\n"
				}
				return fixtureResponse(bodyText), nil
			})}
			stream, err := p.Stream(context.Background(), providers.StreamRequest{Model: ai.Model{ID: "fixture", BaseURL: "https://attacker.test"}, Options: providers.StreamOptions{APIKey: "subscription-token", MaxTokens: 4096}, Context: ai.Context{SystemPrompt: "You are helpful", Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: "hello"}}}}, Tools: []ai.Tool{{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}}}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stream.Close() }()
			for {
				_, err = stream.Next(context.Background())
				if err != nil {
					break
				}
			}
			if test.wantError == "" {
				if !errors.Is(err, io.EOF) {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("wanted %s, got %v", test.wantError, err)
			}
		})
	}
}

func TestSubscriptionRejectsOtherRoutes(t *testing.T) {
	transport := routeTransport{roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unsafe route sent"); return nil, nil })}
	for _, u := range []string{"https://attacker.test/v1/responses", "https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/responses/input_tokens", "http://api.openai.com/v1/responses"} {
		req, _ := http.NewRequest(http.MethodPost, u, strings.NewReader(`{}`))
		resp, err := transport.RoundTrip(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if err == nil {
			t.Fatal("unsafe route accepted", u)
		}
	}
}

func TestSubscriptionReplaysAssistantAsOutputText(t *testing.T) {
	p := NewProvider(nil)
	p.http.Transport = routeTransport{roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Input []struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Input) != 3 || body.Input[1].Role != "assistant" || body.Input[1].Content[0].Type != "output_text" || body.Input[2].Content[0].Text != "second" {
			t.Fatal("invalid multi-turn assistant history")
		}
		return fixtureResponse("data: " + `{"type":"response.completed","response":{"status":"completed"}}` + "\n\n"), nil
	})}
	stream, err := p.Stream(context.Background(), providers.StreamRequest{Model: ai.Model{ID: "fixture"}, Options: providers.StreamOptions{APIKey: "fixture"}, Context: ai.Context{Messages: []ai.Message{
		{Role: ai.RoleUser, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: "first"}}},
		{Role: ai.RoleAssistant, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: "first answer"}}},
		{Role: ai.RoleUser, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: "second"}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
}

func TestSubscriptionHTTPErrorPreservesDiagnosticsAndClassification(t *testing.T) {
	p := NewProvider(nil)
	p.http.Transport = routeTransport{roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"req_fixture"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_value","param":"input[1].content[0]","message":"private-token-echo"}}`))}, nil
	})}
	_, err := p.Stream(context.Background(), providers.StreamRequest{Model: ai.Model{ID: "fixture"}, Options: providers.StreamOptions{APIKey: "fixture"}, Context: ai.Context{Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: "hello"}}}}}})
	var network *providers.NetworkError
	if err == nil || errors.As(err, &network) || !strings.Contains(err.Error(), "invalid_value") || !strings.Contains(err.Error(), "input[1].content[0]") || !strings.Contains(err.Error(), "req_fixture") || strings.Contains(err.Error(), "private-token-echo") {
		t.Fatalf("lost or leaked HTTP diagnostics: %v", err)
	}
}
