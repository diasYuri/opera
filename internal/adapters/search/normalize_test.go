package search

import (
	"strings"
	"testing"

	searchcontract "github.com/diasYuri/y/pkg/search"
)

func TestResultCleansCitationAndEnforcesDomainFilters(t *testing.T) {
	request := searchcontract.Request{AllowedDomains: []string{"example.com"}, ExcludedDomains: []string{"private.example.com"}}
	result, ok := Result("<b>Trusted &amp; cited</b>", "https://www.example.com/source", "A\n safe\t snippet", request)
	if !ok || result.Title != "Trusted & cited" || result.Snippet != "A safe snippet" {
		t.Fatalf("clean citation = %#v, accepted=%v", result, ok)
	}
	for _, rawURL := range []string{"javascript:alert(1)", "https://user@example.com/", "https://private.example.com/report", "https://notexample.com/"} {
		if _, ok := Result("title", rawURL, "", request); ok {
			t.Errorf("accepted unsafe or excluded result URL %q", rawURL)
		}
	}
}

func TestLimitRemovesDuplicatesAndAppliesDefault(t *testing.T) {
	results := []searchcontract.Result{
		{Title: "one", URL: "https://example.com/one"},
		{Title: "duplicate", URL: "https://example.com/one"},
		{Title: "two", URL: "https://example.com/two"},
	}
	if got := Limit(results, 1); len(got) != 1 || got[0].Title != "one" {
		t.Fatalf("bounded result set = %#v", got)
	}
	if got := Limit(results, 0); len(got) != 2 {
		t.Fatalf("default result limit kept duplicates: %#v", got)
	}
}

func TestErrorCodeAcceptsOnlyBoundedIdentifiers(t *testing.T) {
	if got := ErrorCode("rate_limited"); got != "rate_limited" {
		t.Fatalf("safe provider code = %q", got)
	}
	for _, raw := range []string{"token-value", "secret\nvalue", strings.Repeat("x", 65)} {
		if got := ErrorCode(raw); got != "" {
			t.Errorf("accepted unsafe provider error code %q", got)
		}
	}
}
