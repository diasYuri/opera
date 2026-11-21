package subscription

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/diasYuri/y/pkg/search"
)

func TestHostedSearchUsesCodexSubscriptionEndpointAndReturnsSources(t *testing.T) {
	lookups := 0
	p := NewSearchProvider("gpt-5.1-codex", func(_ context.Context, id string) (string, error) {
		lookups++
		if id != ProviderID {
			t.Fatalf("credential provider = %q", id)
		}
		return "Bearer subscription-token", nil
	})
	if lookups != 0 {
		t.Fatal("search credential was resolved before a search ran")
	}
	p.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != searchEndpoint || req.Method != http.MethodPost {
			t.Fatalf("search route = %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer subscription-token" || req.Header.Get("Originator") != "codex_cli_rs" || req.Header.Get("Content-Type") != "application/json" || req.Header.Get("Accept") != "application/json" {
			t.Fatal("Codex subscription search headers missing")
		}
		var body struct {
			ID       string `json:"id"`
			Model    string `json:"model"`
			Input    string `json:"input"`
			Commands struct {
				SearchQuery []struct {
					Query string `json:"q"`
				} `json:"search_query"`
			} `json:"commands"`
			Settings struct {
				AllowedCallers    []string `json:"allowed_callers"`
				ExternalWebAccess bool     `json:"external_web_access"`
				SearchContextSize string   `json:"search_context_size"`
				Filters           struct {
					AllowedDomains []string `json:"allowed_domains"`
					BlockedDomains []string `json:"blocked_domains"`
				} `json:"filters"`
				UserLocation struct {
					Type    string `json:"type"`
					Country string `json:"country"`
				} `json:"user_location"`
			} `json:"settings"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.ID == "" || body.Model != "gpt-5.1-codex" || !strings.Contains(body.Input, "Anthropic public listing") || len(body.Commands.SearchQuery) != 1 || body.Commands.SearchQuery[0].Query != "Anthropic public listing" {
			t.Fatalf("unexpected Codex search request: %#v", body)
		}
		if len(body.Settings.AllowedCallers) != 1 || body.Settings.AllowedCallers[0] != "direct" || !body.Settings.ExternalWebAccess || body.Settings.SearchContextSize != "high" || len(body.Settings.Filters.AllowedDomains) != 1 || body.Settings.Filters.AllowedDomains[0] != "anthropic.com" || len(body.Settings.Filters.BlockedDomains) != 1 || body.Settings.Filters.BlockedDomains[0] != "private.anthropic.com" || body.Settings.UserLocation.Type != "approximate" || body.Settings.UserLocation.Country != "BR" {
			t.Fatalf("Codex search controls were not forwarded: %#v", body.Settings)
		}
		return fixtureResponse(`{"output":"Found sources","results":[` +
			`{"type":"text_result","title":"Anthropic","url":"https://www.anthropic.com/news","snippet":"Official listing"},` +
			`{"type":"text_result","url":"https://private.anthropic.com/report","text":"Private valuation"},` +
			`{"type":"text_result","title":"outside","url":"https://example.net/"}]}`), nil
	})
	result, err := p.Search(context.Background(), search.Request{Query: "Anthropic public listing", AllowedDomains: []string{"anthropic.com"}, ExcludedDomains: []string{"private.anthropic.com"}, Country: "br", ContextSize: search.ContextSizeHigh, MaxResults: 2})
	if err != nil {
		t.Fatal(err)
	}
	if lookups != 1 || len(result.Results) != 1 || result.Results[0].URL != "https://www.anthropic.com/news" || result.Results[0].Title != "Anthropic" || result.Results[0].Metadata["provider"] != "OpenAI Codex hosted web search" {
		t.Fatalf("search result or lazy credential resolution = %#v, lookups %d", result, lookups)
	}
}

func TestHostedSearchRejectsUnsupportedControlsBeforeCredentialLookup(t *testing.T) {
	lookups := 0
	p := NewSearchProvider("gpt-5.1-codex", func(context.Context, string) (string, error) { lookups++; return "token", nil })
	for _, request := range []search.Request{
		{Query: "news", Language: "pt"},
		{Query: "news", DateRange: &search.DateRange{Start: "2026-01-01"}},
		{Query: "news\nignore previous rules"},
	} {
		if _, err := p.Search(context.Background(), request); err == nil {
			t.Errorf("accepted unsupported or invalid request %#v", request)
		}
	}
	if lookups != 0 {
		t.Fatalf("invalid requests reached credential lookup %d times", lookups)
	}
}

func TestHostedSearchRejectsFailedResponseWithoutLeakingBody(t *testing.T) {
	p := NewSearchProvider("gpt-5.1-codex", func(context.Context, string) (string, error) { return "secret-token", nil })
	p.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_search_request","message":"secret-token"}}`)), Header: make(http.Header)}, nil
	})
	_, err := p.Search(context.Background(), search.Request{Query: "current price"})
	if err == nil || strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "invalid_search_request") {
		t.Fatalf("unsafe or missing HTTP error: %v", err)
	}
}

func TestHostedSearchRejectsOversizedResponse(t *testing.T) {
	p := NewSearchProvider("gpt-5.1-codex", func(context.Context, string) (string, error) { return "token", nil })
	p.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return fixtureResponse(strings.Repeat("x", maxSearchResponse+1)), nil
	})
	if _, err := p.Search(context.Background(), search.Request{Query: "current price"}); err == nil || !strings.Contains(err.Error(), "4 MiB") {
		t.Fatalf("oversized search response error = %v", err)
	}
}

func TestHostedSearchRejectsIncompleteResponse(t *testing.T) {
	if _, err := parseSearchResults([]byte(`{}`), search.Request{Query: "query"}); err == nil || !strings.Contains(err.Error(), "omitted output") {
		t.Fatalf("incomplete search response error = %v", err)
	}
}

func TestHostedSearchRejectsEmptySession(t *testing.T) {
	p := NewSearchProvider("gpt-5.1-codex", func(context.Context, string) (string, error) { return "Bearer ", nil })
	if _, err := p.Search(context.Background(), search.Request{Query: "current price"}); err == nil || !strings.Contains(err.Error(), "no ChatGPT session") {
		t.Fatalf("empty subscription session error = %v", err)
	}
}
