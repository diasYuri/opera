package subscription

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	searchadapter "github.com/diasYuri/opera/internal/adapters/search"
	searchcontract "github.com/diasYuri/y/pkg/search"
	"github.com/google/uuid"
)

const (
	searchEndpoint    = "https://chatgpt.com/backend-api/codex/alpha/search"
	maxSearchResponse = 4 << 20
)

var countryCode = regexp.MustCompile(`^[A-Za-z]{2}$`)

// SearchProvider calls Codex's standalone search endpoint with the same
// subscription credential used by the active model provider.
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

func (*SearchProvider) Capabilities() searchcontract.Capabilities {
	return searchcontract.Capabilities{AllowedDomains: true, ExcludedDomains: true, ContextSize: true, MaxResults: true, Country: true}
}

type searchRequest struct {
	ID              string         `json:"id"`
	Model           string         `json:"model"`
	Input           string         `json:"input"`
	Commands        searchCommands `json:"commands"`
	Settings        searchSettings `json:"settings"`
	MaxOutputTokens int            `json:"max_output_tokens"`
}

type searchCommands struct {
	SearchQuery []searchQuery `json:"search_query"`
}

type searchQuery struct {
	Query string `json:"q"`
}

type searchSettings struct {
	UserLocation      *userLocation              `json:"user_location,omitempty"`
	SearchContextSize searchcontract.ContextSize `json:"search_context_size,omitempty"`
	Filters           *domainFilters             `json:"filters,omitempty"`
	AllowedCallers    []string                   `json:"allowed_callers"`
	ExternalWebAccess bool                       `json:"external_web_access"`
}

type domainFilters struct {
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

type userLocation struct {
	Type    string `json:"type"`
	Country string `json:"country"`
}

func (p *SearchProvider) Search(ctx context.Context, request searchcontract.Request) (searchcontract.Response, error) {
	if p == nil || p.lookup == nil {
		return searchcontract.Response{}, errors.New("OpenAI Codex web search requires sign-in with `opera auth login openai-subscription`")
	}
	if ctx == nil {
		return searchcontract.Response{}, errors.New("a context is required for OpenAI web search")
	}
	if err := ctx.Err(); err != nil {
		return searchcontract.Response{}, err
	}
	query := strings.TrimSpace(request.Query)
	if query == "" || len(query) > 512 || strings.ContainsAny(query, "\x00\r\n\x1b") {
		return searchcontract.Response{}, fmt.Errorf("invalid OpenAI web search query: %w", searchcontract.ErrInvalidRequest)
	}
	if request.Location != "" || request.Language != "" || request.DateRange != nil || request.SafeSearch != "" {
		return searchcontract.Response{}, fmt.Errorf("OpenAI hosted web search does not support the requested control: %w", searchcontract.ErrUnsupportedControl)
	}
	if request.MaxResults < 0 || request.MaxResults > 20 {
		return searchcontract.Response{}, fmt.Errorf("max_results for OpenAI web search must be from 0 to 20: %w", searchcontract.ErrInvalidRequest)
	}
	if request.ContextSize != "" && request.ContextSize != searchcontract.ContextSizeLow && request.ContextSize != searchcontract.ContextSizeMedium && request.ContextSize != searchcontract.ContextSizeHigh {
		return searchcontract.Response{}, fmt.Errorf("invalid OpenAI web search context_size: %w", searchcontract.ErrInvalidRequest)
	}
	if request.Country != "" && !countryCode.MatchString(request.Country) {
		return searchcontract.Response{}, fmt.Errorf("country for OpenAI web search must be a two-letter code: %w", searchcontract.ErrInvalidRequest)
	}
	if len(request.AllowedDomains) > 100 || len(request.ExcludedDomains) > 100 {
		return searchcontract.Response{}, fmt.Errorf("OpenAI web search accepts at most 100 domains per filter: %w", searchcontract.ErrInvalidRequest)
	}
	if p.model == "" {
		return searchcontract.Response{}, errors.New("OpenAI web search requires the selected model")
	}
	token, err := p.lookup(ctx, ProviderID)
	if err != nil {
		return searchcontract.Response{}, fmt.Errorf("resolve ChatGPT search session: %w", err)
	}
	token = strings.TrimSpace(token)
	if strings.EqualFold(token, "Bearer") {
		token = ""
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[len("Bearer "):])
	}
	if token == "" {
		return searchcontract.Response{}, errors.New("no ChatGPT session is configured; sign in with `opera auth login openai-subscription`")
	}

	settings := searchSettings{AllowedCallers: []string{"direct"}, ExternalWebAccess: true, SearchContextSize: request.ContextSize}
	if len(request.AllowedDomains) > 0 || len(request.ExcludedDomains) > 0 {
		settings.Filters = &domainFilters{AllowedDomains: request.AllowedDomains, BlockedDomains: request.ExcludedDomains}
	}
	if request.Country != "" {
		settings.UserLocation = &userLocation{Type: "approximate", Country: strings.ToUpper(request.Country)}
	}
	body, err := json.Marshal(searchRequest{
		ID: uuid.NewString(), Model: p.model,
		Input:    "Search the web for this query: " + query,
		Commands: searchCommands{SearchQuery: []searchQuery{{Query: query}}},
		Settings: settings, MaxOutputTokens: 2048,
	})
	if err != nil {
		return searchcontract.Response{}, fmt.Errorf("encode OpenAI web search request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchEndpoint, bytes.NewReader(body))
	if err != nil {
		return searchcontract.Response{}, fmt.Errorf("build OpenAI web search request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := p.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return searchcontract.Response{}, ctx.Err()
		}
		return searchcontract.Response{}, fmt.Errorf("OpenAI hosted web search failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return searchcontract.Response{}, codexSearchHTTPError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSearchResponse+1))
	if err != nil {
		return searchcontract.Response{}, fmt.Errorf("read OpenAI web search response: %w", err)
	}
	if len(data) > maxSearchResponse {
		return searchcontract.Response{}, errors.New("OpenAI web search response exceeded 4 MiB")
	}
	results, err := parseSearchResults(data, request)
	if err != nil {
		return searchcontract.Response{}, err
	}
	for i := range results {
		results[i].Metadata = map[string]string{"provider": "OpenAI Codex hosted web search"}
	}
	return searchcontract.Response{Results: searchadapter.Limit(results, request.MaxResults)}, nil
}

func codexSearchHTTPError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil {
		if code := searchadapter.ErrorCode(body.Error.Code); code != "" {
			return fmt.Errorf("OpenAI hosted web search rejected the request (HTTP %d, %s)", resp.StatusCode, code)
		}
	}
	return fmt.Errorf("OpenAI hosted web search rejected the request (HTTP %d)", resp.StatusCode)
}

func parseSearchResults(data []byte, request searchcontract.Request) ([]searchcontract.Result, error) {
	var response struct {
		Output  *string `json:"output"`
		Results []struct {
			Type    string `json:"type"`
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
			Text    string `json:"text"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, errors.New("OpenAI web search returned an invalid response")
	}
	if response.Output == nil {
		return nil, errors.New("OpenAI web search response omitted output")
	}
	results := make([]searchcontract.Result, 0, len(response.Results))
	for _, item := range response.Results {
		if item.URL == "" || item.Type != "" && item.Type != "text_result" {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			if parsed, err := url.Parse(item.URL); err == nil {
				title = parsed.Hostname()
			}
		}
		snippet := item.Snippet
		if snippet == "" {
			snippet = item.Text
		}
		if result, ok := searchadapter.Result(title, item.URL, snippet, request); ok {
			results = append(results, result)
		}
	}
	return results, nil
}
