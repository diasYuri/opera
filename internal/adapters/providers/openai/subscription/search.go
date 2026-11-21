package subscription

import (
	"bufio"
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
	"unicode"
	"unicode/utf8"

	searchadapter "github.com/diasYuri/opera/internal/adapters/search"
	searchcontract "github.com/diasYuri/y/pkg/search"
)

const (
	searchEndpoint    = "https://api.openai.com/v1/responses"
	searchModel       = "gpt-6-luna"
	maxSearchResponse = 4 << 20
	maxSearchSnippet  = 4 << 10
)

var countryCode = regexp.MustCompile(`^[A-Za-z]{2}$`)

// SearchProvider uses the public Responses API because Sign in with ChatGPT
// credentials are admitted there. ChatGPT's backend-api Codex endpoints are
// outside that OAuth flow's supported endpoint contract.
type SearchProvider struct {
	lookup func(context.Context, string) (string, error)
	client *http.Client
}

func NewSearchProvider(lookup func(context.Context, string) (string, error)) *SearchProvider {
	return &SearchProvider{
		lookup: lookup,
		client: &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

func (*SearchProvider) Capabilities() searchcontract.Capabilities {
	return searchcontract.Capabilities{
		AllowedDomains: true, ExcludedDomains: true, ContextSize: true,
		MaxResults: true, Country: true,
	}
}

type responsesSearchRequest struct {
	Model      string                   `json:"model"`
	Input      []responsesSearchInput   `json:"input"`
	Reasoning  responsesSearchReasoning `json:"reasoning"`
	Tools      []responsesSearchTool    `json:"tools"`
	ToolChoice string                   `json:"tool_choice"`
	Include    []string                 `json:"include"`
	Store      bool                     `json:"store"`
	Stream     bool                     `json:"stream"`
}

type responsesSearchReasoning struct {
	Effort string `json:"effort"`
}

type responsesSearchInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responsesSearchTool struct {
	Type              string                     `json:"type"`
	SearchContextSize searchcontract.ContextSize `json:"search_context_size,omitempty"`
	Filters           *domainFilters             `json:"filters,omitempty"`
	UserLocation      *userLocation              `json:"user_location,omitempty"`
	ExternalWebAccess bool                       `json:"external_web_access"`
}

type domainFilters struct {
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

type userLocation struct {
	Type    string `json:"type"`
	Country string `json:"country,omitempty"`
}

func (p *SearchProvider) Search(ctx context.Context, request searchcontract.Request) (searchcontract.Response, error) {
	if p == nil || p.lookup == nil {
		return searchcontract.Response{}, errors.New("OpenAI web search requires sign-in with `opera auth login openai-subscription`")
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
		return searchcontract.Response{}, fmt.Errorf("OpenAI web search does not support the requested control: %w", searchcontract.ErrUnsupportedControl)
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

	tool := responsesSearchTool{Type: "web_search", SearchContextSize: request.ContextSize, ExternalWebAccess: true}
	if len(request.AllowedDomains) > 0 || len(request.ExcludedDomains) > 0 {
		tool.Filters = &domainFilters{AllowedDomains: request.AllowedDomains, BlockedDomains: request.ExcludedDomains}
	}
	if request.Country != "" {
		tool.UserLocation = &userLocation{Type: "approximate", Country: strings.ToUpper(request.Country)}
	}
	body, err := json.Marshal(responsesSearchRequest{
		Model: searchModel,
		Input: []responsesSearchInput{{
			Role: "user", Content: "Search the web for: " + query + ". Return concise factual findings and cite the sources used.",
		}},
		Reasoning: responsesSearchReasoning{Effort: "low"},
		Tools:     []responsesSearchTool{tool}, ToolChoice: "required",
		Include: []string{"web_search_call.action.sources", "web_search_call.results"},
		Store:   false, Stream: true,
	})
	if err != nil {
		return searchcontract.Response{}, fmt.Errorf("encode OpenAI web search request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchEndpoint, bytes.NewReader(body))
	if err != nil {
		return searchcontract.Response{}, fmt.Errorf("build OpenAI web search request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	client := p.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return searchcontract.Response{}, ctx.Err()
		}
		return searchcontract.Response{}, fmt.Errorf("OpenAI Responses web search failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return searchcontract.Response{}, responsesSearchHTTPError(resp)
	}
	completed, err := readCompletedResponse(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return searchcontract.Response{}, ctx.Err()
		}
		return searchcontract.Response{}, err
	}
	results, err := parseResponsesSearchResults(completed, request)
	if err != nil {
		return searchcontract.Response{}, err
	}
	if len(results) == 0 {
		return searchcontract.Response{}, errors.New("OpenAI Responses web search completed without citeable sources; verify that the selected model and account support web search")
	}
	for i := range results {
		results[i].Metadata = map[string]string{"provider": "OpenAI Responses web search"}
	}
	return searchcontract.Response{Results: searchadapter.Limit(results, request.MaxResults)}, nil
}

func responsesSearchHTTPError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil {
		if code := searchadapter.ErrorCode(body.Error.Code); code != "" {
			return fmt.Errorf("OpenAI Responses web search rejected the request (HTTP %d, %s)", resp.StatusCode, code)
		}
	}
	return fmt.Errorf("OpenAI Responses web search rejected the request (HTTP %d)", resp.StatusCode)
}

// readCompletedResponse consumes the Responses API SSE stream and returns only
// the completed response object. Partial output is never reported as success.
func readCompletedResponse(body io.Reader) ([]byte, error) {
	reader := bufio.NewReader(io.LimitReader(body, maxSearchResponse+1))
	var eventData strings.Builder
	var consumed int
	for {
		line, err := reader.ReadString('\n')
		consumed += len(line)
		if consumed > maxSearchResponse {
			return nil, errors.New("OpenAI web search response exceeded 4 MiB")
		}
		if line == "\n" || line == "\r\n" {
			if eventData.Len() > 0 {
				completed, eventErr := consumeResponseEvent(strings.TrimSuffix(eventData.String(), "\n"))
				if eventErr != nil || completed != nil {
					return completed, eventErr
				}
				eventData.Reset()
			}
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimPrefix(data, " ")
			eventData.WriteString(data)
			eventData.WriteByte('\n')
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("read OpenAI web search stream: %w", err)
			}
			if eventData.Len() > 0 {
				return consumeResponseEvent(strings.TrimSuffix(eventData.String(), "\n"))
			}
			break
		}
	}
	return nil, errors.New("OpenAI web search stream ended before response.completed")
}

func consumeResponseEvent(data string) ([]byte, error) {
	if data == "[DONE]" || data == "" {
		return nil, nil
	}
	var event struct {
		Type     string          `json:"type"`
		Response json.RawMessage `json:"response"`
		Error    struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return nil, errors.New("OpenAI web search returned an invalid event stream")
	}
	switch event.Type {
	case "response.completed":
		if len(event.Response) == 0 || string(event.Response) == "null" {
			return nil, errors.New("OpenAI web search completion omitted its response")
		}
		return event.Response, nil
	case "response.failed", "error":
		code := event.Error.Code
		if code == "" && len(event.Response) > 0 {
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.Unmarshal(event.Response, &response)
			code = response.Error.Code
		}
		if code := searchadapter.ErrorCode(code); code != "" {
			return nil, fmt.Errorf("OpenAI Responses web search failed (%s)", code)
		}
		return nil, errors.New("OpenAI Responses web search failed")
	case "response.incomplete":
		return nil, errors.New("OpenAI Responses web search ended with an incomplete response")
	default:
		return nil, nil
	}
}

type responsesSearchResult struct {
	Type             string `json:"type"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	SourceWebsiteURL string `json:"source_website_url"`
	Snippet          string `json:"snippet"`
	Description      string `json:"description"`
	Text             string `json:"text"`
}

type responsesSearchAnnotation struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	StartIndex  int    `json:"start_index"`
	EndIndex    int    `json:"end_index"`
	URLCitation struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"url_citation"`
}

func parseResponsesSearchResults(data []byte, request searchcontract.Request) ([]searchcontract.Result, error) {
	var response struct {
		Output []struct {
			Type    string                  `json:"type"`
			Status  string                  `json:"status"`
			Results []responsesSearchResult `json:"results"`
			Action  struct {
				Type    string                  `json:"type"`
				URL     string                  `json:"url"`
				Sources []responsesSearchResult `json:"sources"`
			} `json:"action"`
			Content []struct {
				Type        string                      `json:"type"`
				Text        string                      `json:"text"`
				Annotations []responsesSearchAnnotation `json:"annotations"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, errors.New("OpenAI Responses web search returned an invalid response")
	}
	if response.Output == nil {
		return nil, errors.New("OpenAI Responses web search response omitted output")
	}

	type source struct {
		title   string
		url     string
		snippet string
	}
	sources := make([]source, 0)
	byURL := make(map[string]int)
	addSource := func(candidate source) {
		if candidate.url == "" {
			return
		}
		if index, ok := byURL[candidate.url]; ok {
			if sources[index].title == "" {
				sources[index].title = candidate.title
			}
			if sources[index].snippet == "" {
				sources[index].snippet = candidate.snippet
			}
			return
		}
		byURL[candidate.url] = len(sources)
		sources = append(sources, candidate)
	}
	var answer string
	var searchStatus string
	for _, item := range response.Output {
		if item.Type == "web_search_call" {
			searchStatus = item.Status
			for _, found := range append(item.Action.Sources, item.Results...) {
				rawURL := found.URL
				if rawURL == "" {
					rawURL = found.SourceWebsiteURL
				}
				snippet := found.Snippet
				if snippet == "" {
					snippet = found.Text
				}
				if snippet == "" {
					snippet = found.Description
				}
				addSource(source{title: found.Title, url: rawURL, snippet: snippet})
			}
			if item.Action.Type == "open_page" || item.Action.Type == "find_in_page" {
				addSource(source{url: item.Action.URL})
			}
		}
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type != "output_text" {
				continue
			}
			if answer == "" {
				answer = content.Text
			}
			for _, citation := range content.Annotations {
				if citation.Type != "url_citation" {
					continue
				}
				title, rawURL := citation.Title, citation.URL
				if rawURL == "" {
					title, rawURL = citation.URLCitation.Title, citation.URLCitation.URL
				}
				addSource(source{title: title, url: rawURL, snippet: citationContext(content.Text, citation.StartIndex, citation.EndIndex)})
			}
		}
	}

	results := make([]searchcontract.Result, 0, len(sources))
	for _, found := range sources {
		title := strings.TrimSpace(found.title)
		if title == "" {
			if parsed, err := url.Parse(found.url); err == nil {
				title = parsed.Hostname()
			}
		}
		snippet := found.snippet
		if snippet == "" {
			snippet = answer
		}
		snippet = truncateUTF8(strings.TrimSpace(snippet), maxSearchSnippet)
		if result, ok := searchadapter.Result(title, found.url, snippet, request); ok {
			results = append(results, result)
		}
	}
	if len(results) == 0 {
		switch searchStatus {
		case "failed":
			return nil, errors.New("OpenAI Responses web search tool failed; check model and ChatGPT account/workspace web search access")
		case "incomplete":
			return nil, errors.New("OpenAI Responses web search tool ended incomplete")
		}
	}
	return results, nil
}

func citationContext(text string, startIndex, endIndex int) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	start := max(0, min(startIndex, len(runes)))
	end := max(start, min(endIndex, len(runes)))
	left, right := start, end
	for left > 0 && start-left < 600 && !sentenceBoundary(runes[left-1]) {
		left--
	}
	for right < len(runes) && right-end < 600 && !sentenceBoundary(runes[right]) {
		right++
	}
	if right < len(runes) {
		right++
	}
	return strings.TrimSpace(string(runes[left:right]))
}

func sentenceBoundary(r rune) bool {
	return r == '\n' || r == '!' || r == '?' || r == '.' || r == '。' || r == '！' || r == '？'
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimRightFunc(value, unicode.IsSpace)
}
