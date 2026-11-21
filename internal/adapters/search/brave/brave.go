// Package brave adapts Brave Search API to Y's provider-neutral search contract.
package brave

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/diasYuri/y/pkg/search"
)

const endpoint = "https://api.search.brave.com/res/v1/web/search"

const maxResponseBytes = 2 << 20

// Provider performs bounded web searches through Brave Search API.
type Provider struct {
	apiKey   string
	lookup   func(context.Context) (string, error)
	client   *http.Client
	endpoint string
}

// New creates a production provider with a bounded request timeout.
func New(apiKey string) *Provider {
	return &Provider{apiKey: strings.TrimSpace(apiKey), client: &http.Client{
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, endpoint: endpoint}
}

// NewWithCredentialResolver defers access to the user's key until an approved
// search reaches the provider adapter.
func NewWithCredentialResolver(lookup func(context.Context) (string, error)) *Provider {
	provider := New("")
	provider.lookup = lookup
	return provider
}

// Capabilities returns only controls enforced by this adapter.
func (*Provider) Capabilities() search.Capabilities {
	return search.Capabilities{AllowedDomains: true, ExcludedDomains: true, MaxResults: true, Country: true, Language: true, SafeSearch: true}
}

type apiResponse struct {
	Web *struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// Search preserves context cancellation, bounds response memory, and never
// returns provider response bodies that could contain credentials or unsafe text.
func (p *Provider) Search(ctx context.Context, request search.Request) (search.Response, error) {
	if p == nil {
		return search.Response{}, errors.New("missing Brave Search provider")
	}
	if ctx == nil {
		return search.Response{}, errors.New("a context is required for Brave Search")
	}
	if err := ctx.Err(); err != nil {
		return search.Response{}, err
	}
	if request.DateRange != nil {
		return search.Response{}, fmt.Errorf("the Brave Search adapter does not support date_range: %w", search.ErrUnsupportedControl)
	}
	if request.Location != "" || request.ContextSize != "" {
		return search.Response{}, fmt.Errorf("the Brave Search adapter does not support location or context_size: %w", search.ErrUnsupportedControl)
	}
	query := strings.TrimSpace(request.Query)
	if query == "" || len(query) > 512 {
		return search.Response{}, fmt.Errorf("invalid Brave Search query: %w", search.ErrInvalidRequest)
	}
	count := request.MaxResults
	if count == 0 {
		count = 10
	}
	if count < 1 || count > 20 {
		return search.Response{}, fmt.Errorf("max_results for Brave Search must be from 1 to 20: %w", search.ErrInvalidRequest)
	}
	values := url.Values{}
	values.Set("q", query)
	values.Set("count", strconv.Itoa(count))
	if request.Country != "" {
		values.Set("country", strings.ToUpper(request.Country))
	}
	if request.Language != "" {
		values.Set("search_lang", strings.ToLower(request.Language))
	}
	if request.SafeSearch != "" {
		if request.SafeSearch != search.SafeSearchOff && request.SafeSearch != search.SafeSearchModerate && request.SafeSearch != search.SafeSearchStrict {
			return search.Response{}, fmt.Errorf("invalid Brave Search safe_search value: %w", search.ErrInvalidRequest)
		}
		values.Set("safesearch", string(request.SafeSearch))
	}
	apiKey := strings.TrimSpace(p.apiKey)
	if p.lookup != nil {
		var err error
		apiKey, err = p.lookup(ctx)
		if err != nil {
			return search.Response{}, fmt.Errorf("resolve Brave Search credential: %w", err)
		}
		apiKey = strings.TrimSpace(apiKey)
	}
	if apiKey == "" {
		return search.Response{}, errors.New("no Brave Search API key is available; use `opera auth set brave-search.default` or set BRAVE_SEARCH_API_KEY")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"?"+values.Encode(), nil)
	if err != nil {
		return search.Response{}, fmt.Errorf("build Brave Search request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)
	client := p.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return search.Response{}, fmt.Errorf("search request through Brave Search failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return search.Response{}, fmt.Errorf("search service returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return search.Response{}, fmt.Errorf("read Brave Search response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return search.Response{}, errors.New("search response exceeded 2 MiB limit")
	}
	var decoded apiResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return search.Response{}, fmt.Errorf("decode Brave Search response: %w", err)
	}
	if decoded.Web == nil {
		return search.Response{}, errors.New("search response omitted web results")
	}
	results := make([]search.Result, 0, min(count, len(decoded.Web.Results)))
	for _, result := range decoded.Web.Results {
		if !matchesDomains(result.URL, request.AllowedDomains, request.ExcludedDomains) {
			continue
		}
		title, snippet := cleanText(result.Title), cleanText(result.Description)
		if title == "" || len(title) > 1000 || len(snippet) > 5000 || len(result.URL) > 4096 {
			continue
		}
		results = append(results, search.Result{Title: title, URL: result.URL, Snippet: snippet, Metadata: map[string]string{"provider": "Brave Search"}})
		if len(results) == count {
			break
		}
	}
	return search.Response{Results: results}, nil
}

func cleanText(raw string) string {
	// Brave descriptions are text, but may contain encoded entities or markup.
	text := html.UnescapeString(raw)
	var out strings.Builder
	inTag := false
	for _, r := range text {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		case '\n', '\r', '\t':
			if !inTag {
				out.WriteByte(' ')
			}
		default:
			if !inTag && r >= 0x20 && r != 0x7f && r != 0x1b {
				out.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(strings.Join(strings.Fields(out.String()), " "))
}

func matchesDomains(rawURL string, allowed, excluded []string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	for _, domain := range excluded {
		if domainMatches(host, domain) {
			return false
		}
	}
	if len(allowed) == 0 {
		return true
	}
	for _, domain := range allowed {
		if domainMatches(host, domain) {
			return true
		}
	}
	return false
}

func domainMatches(host, domain string) bool {
	domain = strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), "."), ".")
	return domain != "" && (host == domain || strings.HasSuffix(host, "."+domain))
}

// EnvironmentCredential returns the adapter-owned environment variable value.
func EnvironmentCredential(lookup func(string) string) string {
	if lookup == nil {
		return ""
	}
	return strings.TrimSpace(lookup("BRAVE_SEARCH_API_KEY"))
}
