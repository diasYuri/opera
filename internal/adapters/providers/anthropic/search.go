package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	searchadapter "github.com/diasYuri/opera/internal/adapters/search"
	"github.com/diasYuri/y/pkg/search"
)

const (
	searchEndpoint    = "https://api.anthropic.com/v1/messages"
	maxSearchResponse = 4 << 20
)

var searchCountryCode = regexp.MustCompile(`^[A-Za-z]{2}$`)

// SearchProvider invokes Anthropic's hosted web_search tool with the same
// account credential used by the active Claude model.
type SearchProvider struct {
	model  string
	lookup func(context.Context, string) (string, error)
	client *http.Client
}

func NewSearchProvider(model string, lookup func(context.Context, string) (string, error)) *SearchProvider {
	return &SearchProvider{
		model:  model,
		lookup: lookup,
		client: &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

func (*SearchProvider) Capabilities() search.Capabilities {
	return search.Capabilities{AllowedDomains: true, ExcludedDomains: true, MaxResults: true, Country: true}
}

type anthropicSearchRequest struct {
	Model      string                `json:"model"`
	MaxTokens  int                   `json:"max_tokens"`
	Messages   []anthropicMessage    `json:"messages"`
	Tools      []anthropicSearchTool `json:"tools"`
	ToolChoice anthropicToolChoice   `json:"tool_choice"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicSearchTool struct {
	Type          string             `json:"type"`
	Name          string             `json:"name"`
	MaxUses       int                `json:"max_uses"`
	AllowedDomain []string           `json:"allowed_domains,omitempty"`
	BlockedDomain []string           `json:"blocked_domains,omitempty"`
	UserLocation  *anthropicLocation `json:"user_location,omitempty"`
}

type anthropicLocation struct {
	Type    string `json:"type"`
	Country string `json:"country"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func (p *SearchProvider) Search(ctx context.Context, request search.Request) (search.Response, error) {
	if p == nil || p.lookup == nil {
		return search.Response{}, errors.New("claude web search requires sign-in with `opera auth login anthropic`")
	}
	if ctx == nil {
		return search.Response{}, errors.New("a context is required for Claude web search")
	}
	if err := ctx.Err(); err != nil {
		return search.Response{}, err
	}
	query := strings.TrimSpace(request.Query)
	if query == "" || len(query) > 512 || strings.ContainsAny(query, "\x00\r\n\x1b") {
		return search.Response{}, fmt.Errorf("invalid Claude web search query: %w", search.ErrInvalidRequest)
	}
	if request.Location != "" || request.Language != "" || request.DateRange != nil || request.SafeSearch != "" || request.ContextSize != "" {
		return search.Response{}, fmt.Errorf("claude hosted web search does not support the requested control: %w", search.ErrUnsupportedControl)
	}
	if request.MaxResults < 0 || request.MaxResults > 20 {
		return search.Response{}, fmt.Errorf("max_results for Claude web search must be from 0 to 20: %w", search.ErrInvalidRequest)
	}
	if request.Country != "" && !searchCountryCode.MatchString(request.Country) {
		return search.Response{}, fmt.Errorf("country for Claude web search must be a two-letter code: %w", search.ErrInvalidRequest)
	}
	if len(request.AllowedDomains) > 100 || len(request.ExcludedDomains) > 100 {
		return search.Response{}, fmt.Errorf("claude web search accepts at most 100 domains per filter: %w", search.ErrInvalidRequest)
	}
	token, err := p.lookup(ctx, "anthropic")
	if err != nil {
		return search.Response{}, fmt.Errorf("resolve Claude search session: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return search.Response{}, errors.New("no Claude session is configured; use `opera auth login anthropic`")
	}
	token = strings.TrimSpace(token)
	if strings.EqualFold(token, "Bearer") {
		return search.Response{}, errors.New("no Claude session is configured; use `opera auth login anthropic`")
	}
	if p.model == "" {
		return search.Response{}, errors.New("claude web search requires the selected model")
	}
	tool := anthropicSearchTool{Type: "web_search_20250305", Name: "web_search", MaxUses: 1}
	if len(request.AllowedDomains) > 0 {
		// Anthropic rejects a request that supplies both domain fields. Apply the
		// allowed filter remotely and enforce exclusions on returned URLs below.
		tool.AllowedDomain = request.AllowedDomains
	} else {
		tool.BlockedDomain = request.ExcludedDomains
	}
	if request.Country != "" {
		tool.UserLocation = &anthropicLocation{Type: "approximate", Country: strings.ToUpper(request.Country)}
	}
	body, err := json.Marshal(anthropicSearchRequest{
		Model: p.model, MaxTokens: 1024,
		Messages: []anthropicMessage{{Role: "user", Content: "Search the web for this query and return the search results: " + query}},
		Tools:    []anthropicSearchTool{tool}, ToolChoice: anthropicToolChoice{Type: "tool", Name: "web_search"},
	})
	if err != nil {
		return search.Response{}, fmt.Errorf("encode Claude web search request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchEndpoint, bytes.NewReader(body))
	if err != nil {
		return search.Response{}, fmt.Errorf("build Claude web search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		bearerToken := strings.TrimSpace(token[len("Bearer "):])
		if bearerToken == "" {
			return search.Response{}, errors.New("no Claude session is configured; use `opera auth login anthropic`")
		}
		req.Header.Set("Authorization", "Bearer "+bearerToken)
		req.Header.Set("Anthropic-Beta", "oauth-2025-04-20")
	} else {
		req.Header.Set("X-Api-Key", strings.TrimSpace(token))
	}
	client := p.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return search.Response{}, ctx.Err()
		}
		return search.Response{}, fmt.Errorf("claude hosted web search failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return search.Response{}, anthropicSearchHTTPError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSearchResponse+1))
	if err != nil {
		return search.Response{}, fmt.Errorf("read Claude web search response: %w", err)
	}
	if len(data) > maxSearchResponse {
		return search.Response{}, errors.New("claude web search response exceeded 4 MiB")
	}
	results, found, err := parseAnthropicSearchResults(data, request)
	if err != nil {
		return search.Response{}, err
	}
	if !found {
		return search.Response{}, errors.New("claude response completed without a hosted web search result")
	}
	for i := range results {
		results[i].Metadata = map[string]string{"provider": "Anthropic hosted web search"}
	}
	return search.Response{Results: searchadapter.Limit(results, request.MaxResults)}, nil
}

func anthropicSearchHTTPError(resp *http.Response) error {
	return fmt.Errorf("claude hosted web search rejected the request (HTTP %d)", resp.StatusCode)
}

func parseAnthropicSearchResults(data []byte, request search.Request) ([]search.Result, bool, error) {
	var response struct {
		Type    string `json:"type"`
		Content []struct {
			Type    string          `json:"type"`
			Name    string          `json:"name"`
			Content json.RawMessage `json:"content"`
		} `json:"content"`
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, false, errors.New("claude web search returned an invalid response")
	}
	if response.Type == "error" {
		if code := searchadapter.ErrorCode(response.Error.Type); code != "" {
			return nil, false, fmt.Errorf("claude hosted web search failed (%s)", code)
		}
		return nil, false, errors.New("claude hosted web search failed")
	}
	var results []search.Result
	found := false
	for _, block := range response.Content {
		if block.Type != "web_search_tool_result" {
			continue
		}
		if len(block.Content) == 0 || string(block.Content) == "null" {
			return nil, false, errors.New("claude web search returned an incomplete result block")
		}
		found = true
		var items []struct {
			Type      string `json:"type"`
			Title     string `json:"title"`
			URL       string `json:"url"`
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(block.Content, &items); err == nil {
			for _, item := range items {
				if item.Type == "web_search_tool_result_error" {
					if code := searchadapter.ErrorCode(item.ErrorCode); code != "" {
						return nil, false, fmt.Errorf("claude hosted web search failed (%s)", code)
					}
					return nil, false, errors.New("claude hosted web search failed")
				}
				if item.Type != "web_search_result" {
					continue
				}
				if result, ok := searchadapter.Result(item.Title, item.URL, "", request); ok {
					results = append(results, result)
				}
			}
			continue
		}
		var toolError struct {
			Type      string `json:"type"`
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(block.Content, &toolError); err != nil || toolError.Type != "web_search_tool_result_error" {
			return nil, false, errors.New("claude web search returned an invalid result block")
		}
		if code := searchadapter.ErrorCode(toolError.ErrorCode); code != "" {
			return nil, false, fmt.Errorf("claude hosted web search failed (%s)", code)
		}
		return nil, false, errors.New("claude hosted web search failed")
	}
	return results, found, nil
}

var _ search.Provider = (*SearchProvider)(nil)
