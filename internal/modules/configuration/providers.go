package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diasYuri/opera/internal/adapters/platform"
)

var providerNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// CustomProvider is a user-owned provider definition from providers.json.
// Credentials are excluded from JSON serialization of the loaded definition.
type CustomProvider struct {
	Name         string                `json:"name"`
	BaseURL      string                `json:"baseUrl"`
	ProviderType string                `json:"provider_type"`
	APIKey       string                `json:"-"`
	APIKeyHelper string                `json:"-"`
	Models       []CustomProviderModel `json:"models"`
}

func (p CustomProvider) ID() string { return "custom:" + p.Name }

func (p *CustomProvider) UnmarshalJSON(raw []byte) error {
	var value struct {
		Name         string                `json:"name"`
		BaseURL      string                `json:"baseUrl"`
		ProviderType string                `json:"provider_type"`
		APIKey       string                `json:"apikey"`
		APIKeyHelper string                `json:"apikeyhelper"`
		Models       []CustomProviderModel `json:"models"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("invalid provider JSON")
	}
	*p = CustomProvider{Name: value.Name, BaseURL: value.BaseURL, ProviderType: value.ProviderType, APIKey: value.APIKey, APIKeyHelper: value.APIKeyHelper, Models: value.Models}
	return nil
}

type CustomProviderModel struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	ThinkingLevel []string            `json:"thinkingLevel,omitempty"`
	Input         []string            `json:"input"`
	Cost          *CustomProviderCost `json:"cost"`
	ContextWindow int64               `json:"contextWindow"`
	MaxTokens     int64               `json:"maxTokens"`
}

type CustomProviderCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

func LoadCustomProviders(path, configDir string) ([]CustomProvider, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open providers configuration: %w", err)
	}
	defer func() { _ = file.Close() }()
	if configDir != "" {
		if err := platform.Check(configDir, true); err != nil {
			return nil, fmt.Errorf("opera config directory permissions: %w", err)
		}
	}
	if err := platform.Check(path, false); err != nil {
		return nil, fmt.Errorf("providers.json permissions: %w", err)
	}
	if dir := filepath.Dir(path); dir != configDir || configDir == "" {
		if err := platform.Check(dir, true); err != nil {
			return nil, fmt.Errorf("providers directory permissions: %w", err)
		}
	}
	const maxProvidersBytes = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(file, maxProvidersBytes+1))
	if err != nil || len(raw) > maxProvidersBytes {
		return nil, errors.New("cannot read providers.json (limit: 1 MiB)")
	}
	var document struct {
		Providers []CustomProvider `json:"providers"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, errors.New("invalid providers.json; field values omitted")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("invalid providers.json; field values omitted")
	}
	if document.Providers == nil {
		return nil, errors.New("providers.json must contain a providers array")
	}
	seen := make(map[string]struct{}, len(document.Providers))
	for i := range document.Providers {
		provider := &document.Providers[i]
		if err := provider.Validate(); err != nil {
			return nil, fmt.Errorf("providers.json provider %d: %w", i+1, err)
		}
		if _, exists := seen[provider.Name]; exists {
			return nil, fmt.Errorf("providers.json contains duplicate provider name %q", provider.Name)
		}
		seen[provider.Name] = struct{}{}
	}
	return document.Providers, nil
}

func (p CustomProvider) Validate() error {
	if !providerNamePattern.MatchString(p.Name) {
		return errors.New("name must be a lowercase provider identifier (letters, numbers, dot, underscore or hyphen)")
	}
	if p.ProviderType != "openai" && p.ProviderType != "anthropic" {
		return errors.New("provider_type must be openai or anthropic")
	}
	parsed, err := url.Parse(p.BaseURL)
	if err != nil || parsed == nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("baseUrl must be an absolute HTTP(S) URL without user info, query or fragment")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return errors.New("baseUrl must use HTTPS except for loopback hosts")
	}
	if (strings.TrimSpace(p.APIKey) == "") == (strings.TrimSpace(p.APIKeyHelper) == "") {
		return errors.New("configure exactly one of apikey or apikeyhelper")
	}
	if strings.ContainsAny(p.APIKey, "\x00\r\n\x1b") || strings.ContainsAny(p.APIKeyHelper, "\x00\r\n\x1b") {
		return errors.New("credential fields cannot contain control characters")
	}
	if p.APIKeyHelper != "" && !filepath.IsAbs(p.APIKeyHelper) {
		return errors.New("apikeyhelper must be an absolute executable path")
	}
	if len(p.APIKey) > 64*1024 || len(p.APIKeyHelper) > 4096 {
		return errors.New("credential field exceeds its size limit")
	}
	if len(p.Models) == 0 {
		return errors.New("models must contain at least one model")
	}
	seen := make(map[string]struct{}, len(p.Models))
	for _, model := range p.Models {
		if strings.TrimSpace(model.ID) == "" || len(model.ID) > 256 || strings.ContainsAny(model.ID, "\x00\r\n\x1b") {
			return errors.New("each model requires a valid id")
		}
		if strings.TrimSpace(model.Name) == "" || len(model.Name) > 256 || strings.ContainsAny(model.Name, "\x00\r\n\x1b") {
			return errors.New("each model requires a valid name")
		}
		if _, exists := seen[model.ID]; exists {
			return fmt.Errorf("duplicate model id %q", model.ID)
		}
		seen[model.ID] = struct{}{}
		if model.Cost == nil || !validCost(*model.Cost) {
			return fmt.Errorf("model %q requires non-negative cost rates", model.ID)
		}
		if model.ContextWindow < 1 || model.MaxTokens < 1 || model.MaxTokens > model.ContextWindow {
			return fmt.Errorf("model %q requires positive contextWindow/maxTokens with maxTokens no greater than contextWindow", model.ID)
		}
		if len(model.Input) == 0 {
			return fmt.Errorf("model %q input must include text or image", model.ID)
		}
		inputs := map[string]bool{}
		for _, input := range model.Input {
			if input != "text" && input != "image" {
				return fmt.Errorf("model %q input values must be text or image", model.ID)
			}
			if inputs[input] {
				return fmt.Errorf("model %q input contains duplicates", model.ID)
			}
			inputs[input] = true
		}
		levels := map[string]bool{}
		for _, level := range model.ThinkingLevel {
			if !validThinkingLevel(level) || levels[level] {
				return fmt.Errorf("model %q has an invalid or duplicate thinkingLevel", model.ID)
			}
			levels[level] = true
		}
	}
	return nil
}

func validThinkingLevel(level string) bool {
	switch level {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func validCost(cost CustomProviderCost) bool {
	for _, value := range []float64{cost.Input, cost.Output, cost.CacheRead, cost.CacheWrite} {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func isLoopbackHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
