package brave

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diasYuri/y/pkg/search"
)

func TestSearchMapsRequestAndReturnsCiteableResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/search" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Subscription-Token"); got != "test-secret" {
			t.Fatalf("API key header = %q", got)
		}
		for key, want := range map[string]string{"q": "Anthropic valuation", "country": "BR", "search_lang": "pt", "count": "3", "safesearch": "strict"} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"web":{"results":[`+
			`{"title":"Official","url":"https://anthropic.com/news","description":"<b>Company update</b> &amp; details"},`+
			`{"title":"Wrong domain","url":"https://example.org/story","description":"skip me"},`+
			`{"title":"Subdomain","url":"https://invest.anthropic.com/story","description":"Private valuation"},`+
			`{"title":"Excluded","url":"https://blog.anthropic.com/story","description":"skip me"}`+
			`]}}`)
	}))
	defer server.Close()
	provider := New("test-secret")
	provider.client = server.Client()
	provider.endpoint = server.URL + "/search"

	response, err := provider.Search(context.Background(), search.Request{
		Query: "Anthropic valuation", Country: "br", Language: "PT", MaxResults: 3,
		SafeSearch: search.SafeSearchStrict, AllowedDomains: []string{"anthropic.com"}, ExcludedDomains: []string{"blog.anthropic.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(response.Results), response.Results)
	}
	if response.Results[0].Title != "Official" || response.Results[0].Snippet != "Company update & details" {
		t.Fatalf("result text was not cleaned: %#v", response.Results[0])
	}
	if response.Results[1].URL != "https://invest.anthropic.com/story" || response.Results[0].Metadata["provider"] != "Brave Search" {
		t.Fatalf("citation evidence missing: %#v", response.Results)
	}
}

func TestSearchRejectsUnsupportedControlsAndBadResponses(t *testing.T) {
	provider := New("key")
	for _, request := range []search.Request{
		{Query: "query", DateRange: &search.DateRange{Start: "2026-01-01"}},
		{Query: "query", Location: "São Paulo"},
		{Query: "query", MaxResults: 21},
	} {
		if _, err := provider.Search(context.Background(), request); err == nil {
			t.Errorf("accepted unsupported/invalid request %#v", request)
		}
	}
	for _, body := range []string{"not-json", strings.Repeat("x", maxResponseBytes+1)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, body) }))
		provider.client, provider.endpoint = server.Client(), server.URL
		_, err := provider.Search(context.Background(), search.Request{Query: "query"})
		server.Close()
		if err == nil {
			t.Errorf("accepted bad response of %d bytes", len(body))
		}
	}
}

func TestSearchRedactsErrorBodyAndHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, "test-secret invalid credential")
	}))
	defer server.Close()
	provider := New("test-secret")
	provider.client, provider.endpoint = server.Client(), server.URL
	_, err := provider.Search(context.Background(), search.Request{Query: "query"})
	if err == nil || strings.Contains(err.Error(), "test-secret") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("unsafe or missing HTTP error: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.Search(canceled, search.Request{Query: "query"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled search error = %v", err)
	}
}

func TestSearchDoesNotForwardCredentialAcrossRedirect(t *testing.T) {
	var destinationCalled bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationCalled = true
		if got := r.Header.Get("X-Subscription-Token"); got != "" {
			t.Errorf("redirect forwarded API key %q", got)
		}
		_, _ = fmt.Fprint(w, `{"web":{"results":[]}}`)
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", destination.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	provider := New("test-secret")
	provider.endpoint = source.URL
	_, err := provider.Search(context.Background(), search.Request{Query: "query"})
	if err == nil || !strings.Contains(err.Error(), "307") || destinationCalled {
		t.Fatalf("redirect result = %v, destination called=%v", err, destinationCalled)
	}
}

func TestEnvironmentCredential(t *testing.T) {
	if got := EnvironmentCredential(func(key string) string {
		if key == "BRAVE_SEARCH_API_KEY" {
			return " key \n"
		}
		return ""
	}); got != "key" {
		t.Fatalf("credential = %q", got)
	}
	if got := EnvironmentCredential(nil); got != "" {
		t.Fatalf("nil lookup returned %q", got)
	}
}

func TestCredentialResolverRunsOnlyWhenSearchExecutes(t *testing.T) {
	resolved := 0
	provider := NewWithCredentialResolver(func(context.Context) (string, error) {
		resolved++
		return "approved-search-key", nil
	})
	if resolved != 0 {
		t.Fatal("credential was resolved during provider construction")
	}
	if _, err := provider.Search(context.Background(), search.Request{Query: "query", DateRange: &search.DateRange{Start: "2026-01-01"}}); err == nil {
		t.Fatal("unsupported search control was accepted")
	}
	if resolved != 0 {
		t.Fatal("invalid request resolved the credential")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Subscription-Token"); got != "approved-search-key" {
			t.Errorf("request credential = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"web":{"results":[]}}`)
	}))
	defer server.Close()
	provider.client, provider.endpoint = server.Client(), server.URL
	if _, err := provider.Search(context.Background(), search.Request{Query: "query"}); err != nil {
		t.Fatal(err)
	}
	if resolved != 1 {
		t.Fatalf("credential resolver calls = %d, want one", resolved)
	}
}

func TestDomainMatchingIsSubdomainAware(t *testing.T) {
	for _, test := range []struct {
		host string
		want bool
	}{{"https://anthropic.com/a", true}, {"https://invest.anthropic.com/a", true}, {"https://notanthropic.com/a", false}, {"javascript:alert(1)", false}, {"https://u@anthropic.com", false}} {
		if got := matchesDomains(test.host, []string{"anthropic.com"}, nil); got != test.want {
			t.Errorf("matchesDomains(%q) = %v, want %v", test.host, got, test.want)
		}
	}
}

func TestSearchResponseHasNoUnexpectedRawFields(t *testing.T) {
	var result apiResponse
	if err := json.Unmarshal([]byte(`{"web":{"results":[{"title":"x","url":"https://example.com","description":"snippet","extra":"ignored"}]}}`), &result); err != nil {
		t.Fatal(err)
	}
	if result.Web == nil || len(result.Web.Results) != 1 || result.Web.Results[0].Title != "x" {
		t.Fatalf("unexpected decoded results: %#v", result)
	}
}
