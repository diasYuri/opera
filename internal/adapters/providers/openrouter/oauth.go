package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	authorizeURL = "https://openrouter.ai/auth"
	tokenURL     = "https://openrouter.ai/api/v1/auth/keys"
	maxAuthBody  = 1 << 20
)

func EnvironmentCredential(lookup func(string) string) string { return lookup("OPENROUTER_API_KEY") }

func AuthorizationURL(callbackURL, challenge string) string {
	q := url.Values{"callback_url": {callbackURL}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return authorizeURL + "?" + q.Encode()
}

func ParseAuthorizationInput(input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", errors.New("missing OpenRouter authorization code")
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		value = parsed.Query().Get("code")
	} else if strings.Contains(value, "code=") {
		parsed, err := url.ParseQuery(value)
		if err != nil {
			return "", errors.New("invalid OpenRouter authorization response")
		}
		value = parsed.Get("code")
	}
	if value == "" || hasForbiddenControl(value) {
		return "", errors.New("invalid OpenRouter authorization code")
	}
	return value, nil
}

func Exchange(ctx context.Context, code, verifier string) (string, error) {
	var result struct {
		Key string `json:"key"`
	}
	if err := postOAuthJSON(ctx, map[string]string{"code": code, "code_verifier": verifier, "code_challenge_method": "S256"}, &result); err != nil {
		return "", fmt.Errorf("OpenRouter token exchange failed: %w", err)
	}
	if result.Key == "" {
		return "", errors.New("OpenRouter token exchange returned no API key")
	}
	return result.Key, nil
}

func hasForbiddenControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func postOAuthJSON(ctx context.Context, body any, result any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return errors.New("cannot encode provider authentication request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(string(raw)))
	if err != nil {
		return errors.New("cannot create provider authentication request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 || next.URL.Scheme != "https" || next.URL.Host != req.URL.Host {
			return errors.New("authentication endpoint redirected outside its origin")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("provider authentication endpoint is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider authentication endpoint returned HTTP %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, maxAuthBody))
	if err := decoder.Decode(result); err != nil {
		return errors.New("provider authentication response is invalid")
	}
	return nil
}
