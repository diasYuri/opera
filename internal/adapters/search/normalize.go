// Package search contains shared result validation for provider-owned search
// adapters. Network clients and provider protocols live in child packages.
package search

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	searchcontract "github.com/diasYuri/y/pkg/search"
)

var safeErrorCode = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)

const (
	maxTitleBytes   = 1000
	maxSnippetBytes = 5000
	maxURLBytes     = 4096
)

// Result sanitizes a provider result and enforces the caller's domain filters.
func Result(title, rawURL, snippet string, request searchcontract.Request) (searchcontract.Result, bool) {
	title = cleanText(title)
	snippet = cleanText(snippet)
	if title == "" || len(title) > maxTitleBytes || len(snippet) > maxSnippetBytes || len(rawURL) > maxURLBytes {
		return searchcontract.Result{}, false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || hasControl(rawURL) {
		return searchcontract.Result{}, false
	}
	if !matchesDomains(strings.ToLower(strings.TrimSuffix(parsed.Hostname(), ".")), request.AllowedDomains, request.ExcludedDomains) {
		return searchcontract.Result{}, false
	}
	return searchcontract.Result{Title: title, URL: rawURL, Snippet: snippet}, true
}

// Limit removes duplicate URLs and applies the output bound requested by the
// caller. The default is ten results.
func Limit(results []searchcontract.Result, requested int) []searchcontract.Result {
	if requested <= 0 {
		requested = 10
	}
	seen := make(map[string]struct{}, min(len(results), requested))
	out := make([]searchcontract.Result, 0, min(len(results), requested))
	for _, result := range results {
		parsed, err := url.Parse(result.URL)
		if err != nil {
			continue
		}
		key := parsed.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, result)
		if len(out) == requested {
			break
		}
	}
	return out
}

// ErrorCode keeps provider-controlled error metadata from becoming arbitrary
// diagnostic text.
func ErrorCode(value string) string {
	if !safeErrorCode.MatchString(value) {
		return ""
	}
	return value
}

func cleanText(raw string) string {
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
			if !inTag && !unicode.IsControl(r) {
				out.WriteRune(r)
			}
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func hasControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func matchesDomains(host string, allowed, excluded []string) bool {
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
