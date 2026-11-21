// Package config loads core settings and user-owned provider definitions.
package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Field struct {
	Key     string   `json:"key"`
	Kind    string   `json:"kind"`
	Default any      `json:"default"`
	Choices []string `json:"choices,omitempty"`
	Min     int      `json:"min,omitempty"`
	Max     int      `json:"max,omitempty"`
}

// Schema is also the source of truth for environment variables and --set.
func Schema() []Field {
	fields := []Field{
		{Key: "accounting.prices_file", Kind: "string", Default: ""},
		{Key: "extensions.file", Kind: "string", Default: ""},
		{Key: "provider", Kind: "string", Default: ""},
		{Key: "model", Kind: "string", Default: ""},
		{Key: "search.provider", Kind: "string", Default: "", Choices: []string{"", "native", "brave", "disabled"}},
		{Key: "thinking", Kind: "string", Default: "off", Choices: []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}},
		{Key: "base_url", Kind: "string", Default: ""},
		{Key: "policy", Kind: "string", Default: "ask", Choices: []string{"read-only", "ask", "trusted"}},
		{Key: "credential_ref", Kind: "string", Default: "default"},
		{Key: "credentials.store", Kind: "string", Default: "keyring", Choices: []string{"keyring", "file"}},
		{Key: "output.format", Kind: "string", Default: "text", Choices: []string{"text", "jsonl"}},
		{Key: "ui.no_motion", Kind: "bool", Default: false},
		{Key: "ui.accessible", Kind: "bool", Default: false},
		{Key: "ui.theme", Kind: "string", Default: "system", Choices: []string{"system", "dark", "light", "midnight", "ocean", "forest", "sunset", "mono"}},
		{Key: "agents.max_concurrent", Kind: "int", Default: 4, Min: 1, Max: 32},
		{Key: "agents.max_depth", Kind: "int", Default: 1, Min: 0, Max: 8},
		{Key: "limits.worker_tokens", Kind: "int", Default: 0, Min: 0, Max: 10000000},
		{Key: "limits.context_tokens", Kind: "int", Default: 64000, Min: 1024, Max: 2000000},
		{Key: "limits.output_bytes", Kind: "int", Default: 1048576, Min: 1024, Max: 67108864},
		{Key: "limits.attachment_bytes", Kind: "int", Default: 10485760, Min: 1024, Max: 104857600},
		{Key: "limits.process_timeout", Kind: "duration", Default: "2m"},
		{Key: "limits.shutdown_timeout", Kind: "duration", Default: "10s"},
	}
	return append(fields, searchOptionFields()...)
}

func EnvironmentKey(key string) string {
	return "OPERA_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

func (f Field) parse(raw string) (any, error) {
	switch f.Kind {
	case "bool":
		value, err := strconv.ParseBool(raw)
		if err == nil {
			return value, nil
		}
	case "int":
		value, err := strconv.Atoi(raw)
		if err == nil && value >= f.Min && value <= f.Max {
			return value, nil
		}
	case "duration":
		value, err := time.ParseDuration(raw)
		if err == nil && value > 0 && value <= 24*time.Hour {
			return value.String(), nil
		}
	case "string":
		if strings.ContainsAny(raw, "\x00\r\n\x1b") || len(raw) > 512 || (len(f.Choices) > 0 && !containsChoice(f.Choices, raw)) {
			break
		}
		switch {
		case strings.HasSuffix(f.Key, ".country"):
			if raw != "" && !validTwoLetterCode(raw) {
				break
			}
			return strings.ToUpper(raw), nil
		case strings.HasSuffix(f.Key, ".language"):
			if raw != "" && !validTwoLetterCode(raw) {
				break
			}
			return strings.ToLower(raw), nil
		case strings.HasSuffix(f.Key, ".allowed_domains"), strings.HasSuffix(f.Key, ".excluded_domains"):
			return normalizeSearchDomains(raw)
		default:
			return raw, nil
		}
	}
	// Values can be sensitive, even if supplied to the wrong field.
	return nil, fmt.Errorf("invalid value for %s (expected %s)", f.Key, f.Kind)
}

func containsChoice(choices []string, value string) bool {
	for _, choice := range choices {
		if choice == value {
			return true
		}
	}
	return false
}
