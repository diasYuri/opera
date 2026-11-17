package anthropic

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
	oauthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	authorizeURL  = "https://claude.ai/oauth/authorize"
	tokenURL      = "https://platform.claude.com/v1/oauth/token"
	codeReturnURL = "https://platform.claude.com/oauth/code/callback"
	maxAuthBody   = 1 << 20
)

type OAuthToken struct {
	AccessToken, RefreshToken string
	ExpiresAt                 time.Time
}
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// EnvironmentCredential preserves Anthropic's bearer-token and API-key
// environment conventions for credential lookup and interactive login.
func EnvironmentCredential(lookup func(string) string) string {
	for _, key := range []struct {
		name   string
		bearer bool
	}{{"ANTHROPIC_AUTH_TOKEN", true}, {"ANTHROPIC_OAUTH_TOKEN", true}, {"ANTHROPIC_API_KEY", false}} {
		if value := lookup(key.name); value != "" {
			if key.bearer {
				return "Bearer " + value
			}
			return value
		}
	}
	return ""
}

// AuthorizationURL builds Claude's provider-specific authorization request.
func AuthorizationURL(redirectURI, challenge, state string) string {
	q := url.Values{"code": {"true"}, "client_id": {oauthClientID}, "response_type": {"code"}, "redirect_uri": {redirectURI}, "scope": {"org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "state": {state}}
	return authorizeURL + "?" + q.Encode()
}

func CodeReturnURL() string { return codeReturnURL }

func ParseAuthorizationInput(input, expectedState string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", errors.New("missing Anthropic authorization code")
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		code, state := parsed.Query().Get("code"), parsed.Query().Get("state")
		if code == "" || state != expectedState {
			return "", errors.New("Anthropic authorization response is incomplete or has invalid state")
		}
		return code, nil
	}
	if strings.Contains(value, "#") {
		parts := strings.SplitN(value, "#", 2)
		if parts[1] != expectedState || parts[0] == "" {
			return "", errors.New("Anthropic authorization response has invalid state")
		}
		return parts[0], nil
	}
	if strings.Contains(value, "code=") {
		parsed, err := url.ParseQuery(value)
		if err != nil || parsed.Get("code") == "" || parsed.Get("state") != expectedState {
			return "", errors.New("Anthropic authorization response is incomplete or has invalid state")
		}
		return parsed.Get("code"), nil
	}
	return "", errors.New("Anthropic authorization code must include its state")
}

func Exchange(ctx context.Context, code, verifier, redirectURI string) (OAuthToken, error) {
	body := map[string]string{"grant_type": "authorization_code", "client_id": oauthClientID, "code": code, "state": verifier, "redirect_uri": redirectURI, "code_verifier": verifier}
	var token tokenResponse
	if err := postOAuthJSON(ctx, body, &token); err != nil {
		return OAuthToken{}, fmt.Errorf("Anthropic token exchange failed: %w", err)
	}
	return normalizeToken(token, "authorization response", true)
}

func Refresh(ctx context.Context, refreshToken string) (OAuthToken, error) {
	var token tokenResponse
	if err := postOAuthJSON(ctx, map[string]string{"grant_type": "refresh_token", "client_id": oauthClientID, "refresh_token": refreshToken}, &token); err != nil {
		return OAuthToken{}, fmt.Errorf("Anthropic token refresh failed: %w", err)
	}
	result, err := normalizeToken(token, "refresh response", false)
	if err == nil && result.RefreshToken == "" {
		result.RefreshToken = refreshToken
	}
	return result, err
}

func normalizeToken(token tokenResponse, source string, requireRefresh bool) (OAuthToken, error) {
	if token.AccessToken == "" || requireRefresh && token.RefreshToken == "" || token.ExpiresIn <= 0 {
		return OAuthToken{}, fmt.Errorf("Anthropic %s omitted required token fields", source)
	}
	expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	if token.ExpiresIn > 300 {
		expiresAt = expiresAt.Add(-5 * time.Minute)
	}
	return OAuthToken{token.AccessToken, token.RefreshToken, expiresAt}, nil
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
	limited := http.MaxBytesReader(nil, resp.Body, maxAuthBody)
	if err := json.NewDecoder(limited).Decode(result); err != nil {
		return errors.New("provider authentication response is invalid")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider authentication endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}
