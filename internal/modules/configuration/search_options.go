package config

import (
	"fmt"
	"strings"
)

type WebSearchOption struct {
	Key, Label, Description string
	InputKind, ValueKind    string
	Default                 any
	Choices                 []string
	Min, Max                int
}

type WebSearchDefaults struct {
	AllowedDomains  []string
	ExcludedDomains []string
	ContextSize     string
	Country         string
	Language        string
	SafeSearch      string
	MaxResults      int
}

// These IDs own persisted option keys. Discover installed search backends
// through the composition catalog.
var searchOptionProviderIDs = [...]string{"openai-subscription", "anthropic", "brave"}

func WebSearchOptions(providerID string) []WebSearchOption {
	common := []WebSearchOption{
		{Key: "max_results", Label: "Maximum results", Description: "0 keeps the provider default; otherwise limit results to this count (1–20)", InputKind: "integer", ValueKind: "int", Default: 0, Min: 0, Max: 20},
		{Key: "country", Label: "Country", Description: "Two-letter country code; blank uses the provider default", InputKind: "text", ValueKind: "string", Default: ""},
		{Key: "allowed_domains", Label: "Allowed domains", Description: "Comma-separated hostnames; blank allows any domain", InputKind: "text", ValueKind: "string", Default: ""},
		{Key: "excluded_domains", Label: "Excluded domains", Description: "Comma-separated hostnames to omit", InputKind: "text", ValueKind: "string", Default: ""},
	}
	var options []WebSearchOption
	switch providerID {
	case "openai-subscription":
		options = append(options, WebSearchOption{Key: "context_size", Label: "Search context size", Description: "Amount of context returned from hosted search", InputKind: "select", ValueKind: "string", Default: "", Choices: []string{"", "low", "medium", "high"}})
		options = append(options, common...)
	case "anthropic":
		options = append(options, common...)
	case "brave":
		options = append(options, common...)
		options = append(options,
			WebSearchOption{Key: "language", Label: "Language", Description: "Two-letter language code; blank uses the provider default", InputKind: "text", ValueKind: "string", Default: ""},
			WebSearchOption{Key: "safe_search", Label: "Safe search", Description: "Filter explicit content in Brave results", InputKind: "select", ValueKind: "string", Default: "", Choices: []string{"", "off", "moderate", "strict"}},
		)
	default:
		return nil
	}
	for i := range options {
		options[i].Key = "search.options." + providerID + "." + options[i].Key
		options[i].Choices = append([]string(nil), options[i].Choices...)
	}
	return options
}

func searchOptionFields() []Field {
	var fields []Field
	for _, id := range searchOptionProviderIDs {
		for _, option := range WebSearchOptions(id) {
			fields = append(fields, Field{Key: option.Key, Kind: option.ValueKind, Default: option.Default, Choices: option.Choices, Min: option.Min, Max: option.Max})
		}
	}
	return fields
}

func isWebSearchPreference(key string) bool {
	if key == "search.provider" {
		return true
	}
	for _, id := range searchOptionProviderIDs {
		for _, option := range WebSearchOptions(id) {
			if option.Key == key {
				return true
			}
		}
	}
	return false
}

func (r Resolved) WebSearchDefaults(providerID string) WebSearchDefaults {
	prefix := "search.options." + providerID + "."
	return WebSearchDefaults{
		AllowedDomains:  splitSearchDomains(r.String(prefix + "allowed_domains")),
		ExcludedDomains: splitSearchDomains(r.String(prefix + "excluded_domains")),
		ContextSize:     r.String(prefix + "context_size"),
		Country:         r.String(prefix + "country"),
		Language:        r.String(prefix + "language"),
		SafeSearch:      r.String(prefix + "safe_search"),
		MaxResults:      r.Int(prefix + "max_results"),
	}
}

func splitSearchDomains(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func validTwoLetterCode(value string) bool {
	if len(value) != 2 {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func normalizeSearchDomains(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 100 {
		return "", fmt.Errorf("invalid value for search domain list (maximum 100 domains)")
	}
	normalized := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		domain := strings.ToLower(strings.TrimSpace(part))
		if !validSearchDomain(domain) {
			return "", fmt.Errorf("invalid value for search domain list (use comma-separated hostnames)")
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		normalized = append(normalized, domain)
	}
	return strings.Join(normalized, ","), nil
}

func validSearchDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}
