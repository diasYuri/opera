package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/diasYuri/y/pkg/search"
)

type searchRoundTripFunc func(*http.Request) (*http.Response, error)

func (f searchRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestHostedSearchUsesClaudeOAuthAndReturnsSources(t *testing.T) {
	lookups := 0
	p := NewSearchProvider("claude-sonnet-4-5", func(_ context.Context, provider string) (string, error) {
		lookups++
		if provider != "anthropic" {
			t.Fatalf("credential provider = %q", provider)
		}
		return "Bearer claude-oauth-token", nil
	})
	if lookups != 0 {
		t.Fatal("search credential was resolved before a search ran")
	}
	p.client.Transport = searchRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != searchEndpoint || req.Method != http.MethodPost {
			t.Fatalf("search route = %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer claude-oauth-token" || req.Header.Get("Anthropic-Beta") != "oauth-2025-04-20" || req.Header.Get("Anthropic-Version") != "2023-06-01" {
			t.Fatal("Claude OAuth headers missing")
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools      []anthropicSearchTool `json:"tools"`
			ToolChoice anthropicToolChoice   `json:"tool_choice"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "claude-sonnet-4-5" || len(body.Messages) != 1 || !strings.Contains(body.Messages[0].Content, "Anthropic public listing") || body.ToolChoice.Type != "tool" || body.ToolChoice.Name != "web_search" {
			t.Fatalf("unexpected Claude search request: %#v", body)
		}
		if len(body.Tools) != 1 || body.Tools[0].Type != "web_search_20250305" || body.Tools[0].Name != "web_search" || body.Tools[0].MaxUses != 1 || len(body.Tools[0].AllowedDomain) != 1 || body.Tools[0].AllowedDomain[0] != "anthropic.com" || len(body.Tools[0].BlockedDomain) != 0 || body.Tools[0].UserLocation == nil || body.Tools[0].UserLocation.Country != "BR" {
			t.Fatalf("hosted search controls were not forwarded: %#v", body.Tools)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"type":"message","content":[{"type":"server_tool_use","name":"web_search"},{"type":"web_search_tool_result","content":[{"type":"web_search_result","title":"Anthropic","url":"https://www.anthropic.com/news"},{"type":"web_search_result","title":"private subdomain","url":"https://private.anthropic.com/report"},{"type":"web_search_result","title":"outside","url":"https://example.net/"}]}]}`))}, nil
	})
	result, err := p.Search(context.Background(), search.Request{Query: "Anthropic public listing", AllowedDomains: []string{"anthropic.com"}, ExcludedDomains: []string{"private.anthropic.com"}, Country: "br", MaxResults: 1})
	if err != nil {
		t.Fatal(err)
	}
	if lookups != 1 || len(result.Results) != 1 || result.Results[0].URL != "https://www.anthropic.com/news" {
		t.Fatalf("search result or lazy credential resolution = %#v, lookups %d", result, lookups)
	}
}

func TestHostedSearchParsesSearchToolErrorsWithoutEchoingContent(t *testing.T) {
	for _, test := range []struct {
		body string
		want string
	}{
		{`{"type":"message","content":[{"type":"web_search_tool_result","content":{"type":"web_search_tool_result_error","error_code":"too_many_requests"}}]}`, "too_many_requests"},
		{`{"type":"message","content":[{"type":"web_search_tool_result","content":{"type":"web_search_tool_result_error","error_code":"secret-value"}}]}`, "claude hosted web search failed"},
	} {
		_, _, err := parseAnthropicSearchResults([]byte(test.body), search.Request{Query: "fixture"})
		if err == nil || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), "secret-value") {
			t.Errorf("search error = %v, want safe %q", err, test.want)
		}
	}
}

func TestHostedSearchUsesAPIKeyHeaderForAnthropicAPIKeyLogin(t *testing.T) {
	p := NewSearchProvider("claude-sonnet-4-5", func(context.Context, string) (string, error) { return "api-key-value", nil })
	p.client.Transport = searchRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("X-Api-Key") != "api-key-value" || req.Header.Get("Authorization") != "" {
			t.Fatal("API key was not passed as X-Api-Key")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"type":"message","content":[{"type":"web_search_tool_result","content":[]}]}`))}, nil
	})
	if _, err := p.Search(context.Background(), search.Request{Query: "Anthropic"}); err != nil {
		t.Fatal(err)
	}
}

func TestHostedSearchRejectsUnsupportedControlsBeforeCredentialLookup(t *testing.T) {
	lookups := 0
	p := NewSearchProvider("claude-sonnet-4-5", func(context.Context, string) (string, error) { lookups++; return "token", nil })
	_, err := p.Search(context.Background(), search.Request{Query: "news", Language: "pt"})
	if err == nil || lookups != 0 {
		t.Fatalf("unsupported request reached credential lookup: %v, lookups=%d", err, lookups)
	}
}

func TestHostedSearchDoesNotExposeErrorResponseBody(t *testing.T) {
	p := NewSearchProvider("claude-sonnet-4-5", func(context.Context, string) (string, error) { return "secret-token", nil })
	p.client.Transport = searchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"type":"secret-token"}}`))}, nil
	})
	_, err := p.Search(context.Background(), search.Request{Query: "current price"})
	if err == nil || strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("unsafe or missing HTTP error: %v", err)
	}
}
