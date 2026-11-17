package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/diasYuri/opera/internal/adapters/credentials"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/providers/auth"
)

const (
	providerLoginTimeout = 5 * time.Minute
	providerAuthBodyMax  = 1 << 20
	anthropicClientID    = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	anthropicAuthorize   = "https://claude.ai/oauth/authorize"
	anthropicToken       = "https://platform.claude.com/v1/oauth/token"
	anthropicCodeReturn  = "https://platform.claude.com/oauth/code/callback"
	openRouterAuthorize  = "https://openrouter.ai/auth"
	openRouterToken      = "https://openrouter.ai/api/v1/auth/keys"
)

func (s *chatSession) apiKeyLogin(ctx context.Context, cfg config.Resolved, provider string, notify func(execution.LoginNotice) error) error {
	environmentKey, err := s.environmentAPIKey(ctx, provider)
	if err != nil {
		return err
	}
	if environmentKey != "" {
		return nil
	}
	if cfg.Bool("ui.accessible") {
		store, err := s.app.OpenStore(cfg.String("credentials.store"), cfg.Paths.Credentials)
		if err != nil {
			return err
		}
		key, err := store.Get(provider + "." + cfg.String("credential_ref"))
		if err == nil && key != "" {
			return nil
		}
		if err == nil {
			err = credentials.ErrNotFound
		}
		if !errors.Is(err, credentials.ErrNotFound) && !errors.Is(err, credentials.ErrUnavailable) {
			return err
		}
		return fmt.Errorf("accessible mode does not echo secrets; set the key with opera auth set %s.%s from piped stdin, then run /login again", provider, cfg.String("credential_ref"))
	}
	key, err := promptLoginValue(ctx, notify, execution.LoginPrompt{
		Kind: "secret", Title: "API key", Message: "Enter the provider API key", Placeholder: "API key",
	})
	if err != nil {
		return err
	}
	store, err := s.openLoginStore(ctx, cfg, cfg.String("credentials.store"), cfg.Paths.Credentials, notify)
	if err != nil {
		return err
	}
	if err := store.Set(provider+"."+cfg.String("credential_ref"), key); err != nil {
		return err
	}
	return deleteOptionalCredential(store, oauthReference(provider, cfg.String("credential_ref")))
}

func (s *chatSession) environmentAPIKey(ctx context.Context, provider string) (string, error) {
	if provider == "anthropic" {
		for _, key := range []struct {
			name   string
			bearer bool
		}{{"ANTHROPIC_AUTH_TOKEN", true}, {"ANTHROPIC_OAUTH_TOKEN", true}, {"ANTHROPIC_API_KEY", false}} {
			if value, ok := s.app.Env(key.name); ok && value != "" {
				if key.bearer {
					return "Bearer " + value, nil
				}
				return value, nil
			}
		}
	}
	source := auth.NewEnvSource().WithLookup(func(key string) string {
		value, _ := s.app.Env(key)
		return value
	})
	return source.Resolve(ctx, provider)
}

func promptLoginValue(ctx context.Context, notify func(execution.LoginNotice) error, prompt execution.LoginPrompt) (string, error) {
	response := make(chan string, 1)
	prompt.Response = response
	if err := notify(execution.LoginNotice{Message: prompt.Message, Prompt: &prompt}); err != nil {
		return "", err
	}
	select {
	case value := <-response:
		if strings.TrimSpace(value) == "" {
			return "", errors.New("sign-in value is empty")
		}
		return value, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func promptLoginChoice(ctx context.Context, notify func(execution.LoginNotice) error, title, message string, options []execution.LoginOption) (string, error) {
	response := make(chan string, 1)
	prompt := execution.LoginPrompt{Kind: "select", Title: title, Message: message, Options: options, Response: response}
	if err := notify(execution.LoginNotice{Message: message, Prompt: &prompt}); err != nil {
		return "", err
	}
	select {
	case value := <-response:
		for _, option := range options {
			if option.ID == value {
				return value, nil
			}
		}
		return "", errors.New("invalid sign-in choice")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *chatSession) anthropicOAuthLogin(ctx context.Context, cfg config.Resolved, notify func(execution.LoginNotice) error) error {
	method := "browser"
	if !cfg.Bool("ui.accessible") {
		var err error
		method, err = promptLoginChoice(ctx, notify, "Claude sign-in", "Choose a Claude account login method.", []execution.LoginOption{
			{ID: "browser", Name: "Browser login", Description: "Local callback on this machine"},
			{ID: "copy-code", Name: "Copy code", Description: "Paste the code from the browser"},
		})
		if err != nil {
			return err
		}
	}
	var err error
	loginCtx, cancel := context.WithTimeout(ctx, providerLoginTimeout)
	defer cancel()
	verifier, challenge, err := newPKCEPair()
	if err != nil {
		return errors.New("cannot prepare Anthropic sign-in")
	}
	redirectURI := anthropicCodeReturn
	var callback *loopbackOAuthCallback
	if method == "browser" {
		callback, err = startLoopbackOAuthCallback(loginCtx, "localhost", "/callback", verifier)
		if err != nil {
			return fmt.Errorf("start Anthropic sign-in callback: %w", err)
		}
		defer callback.Close()
		redirectURI = callback.RedirectURI
	} else if method != "copy-code" {
		return errors.New("unknown Anthropic sign-in method")
	}

	query := url.Values{
		"code":                  {"true"},
		"client_id":             {anthropicClientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {"org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {verifier},
	}
	if err := notify(execution.LoginNotice{URL: anthropicAuthorize + "?" + query.Encode(), Message: "Complete Anthropic sign-in in the browser."}); err != nil {
		return err
	}
	var code string
	if method == "browser" {
		code, err = callback.Wait(loginCtx)
	} else {
		var input string
		input, err = promptLoginValue(loginCtx, notify, execution.LoginPrompt{
			Kind: "secret", Title: "Claude authorization code", Message: "Paste the code or redirect URL shown after signing in", Placeholder: "authorization code",
		})
		if err == nil {
			code, err = parseAnthropicAuthorizationInput(input, verifier)
		}
	}
	if err != nil {
		return err
	}
	token, err := exchangeAnthropicCode(loginCtx, code, verifier, redirectURI)
	if err != nil {
		return err
	}
	record := oauthRecord{
		Provider: "anthropic", Flow: "anthropic", AccessToken: token.AccessToken,
		RefreshToken: token.RefreshToken, TokenType: "Bearer", ExpiresAt: token.ExpiresAt,
	}
	return s.storeProviderOAuth(ctx, cfg, "anthropic", record, notify)
}

func parseAnthropicAuthorizationInput(input, expectedState string) (string, error) {
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

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type providerOAuthToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func exchangeAnthropicCode(ctx context.Context, code, verifier, redirectURI string) (providerOAuthToken, error) {
	body := map[string]string{
		"grant_type": "authorization_code", "client_id": anthropicClientID,
		"code": code, "state": verifier, "redirect_uri": redirectURI, "code_verifier": verifier,
	}
	var token oauthTokenResponse
	if err := postProviderJSON(ctx, anthropicToken, body, &token); err != nil {
		return providerOAuthToken{}, fmt.Errorf("Anthropic token exchange failed: %w", err)
	}
	return normalizeAnthropicToken(token, "authorization response", true)
}

func refreshAnthropicToken(ctx context.Context, record oauthRecord) (oauthRecord, error) {
	var token oauthTokenResponse
	body := map[string]string{
		"grant_type": "refresh_token", "client_id": anthropicClientID, "refresh_token": record.RefreshToken,
	}
	if err := postProviderJSON(ctx, anthropicToken, body, &token); err != nil {
		return oauthRecord{}, fmt.Errorf("Anthropic token refresh failed: %w", err)
	}
	normalized, err := normalizeAnthropicToken(token, "refresh response", false)
	if err != nil {
		return oauthRecord{}, err
	}
	if normalized.RefreshToken == "" {
		normalized.RefreshToken = record.RefreshToken
	}
	record.AccessToken, record.RefreshToken, record.ExpiresAt = normalized.AccessToken, normalized.RefreshToken, normalized.ExpiresAt
	return record, nil
}

func normalizeAnthropicToken(token oauthTokenResponse, source string, requireRefresh bool) (providerOAuthToken, error) {
	if token.AccessToken == "" || requireRefresh && token.RefreshToken == "" || token.ExpiresIn <= 0 {
		return providerOAuthToken{}, fmt.Errorf("Anthropic %s omitted required token fields", source)
	}
	expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	if token.ExpiresIn > 5*60 {
		expiresAt = expiresAt.Add(-5 * time.Minute)
	}
	return providerOAuthToken{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, ExpiresAt: expiresAt}, nil
}

func (s *chatSession) openRouterOAuthLogin(ctx context.Context, cfg config.Resolved, notify func(execution.LoginNotice) error) error {
	loginCtx, cancel := context.WithTimeout(ctx, providerLoginTimeout)
	defer cancel()
	verifier, challenge, err := newPKCEPair()
	if err != nil {
		return errors.New("cannot prepare OpenRouter sign-in")
	}
	pathToken, err := randomURLToken(18)
	if err != nil {
		return errors.New("cannot prepare OpenRouter sign-in")
	}
	callbackPath := "/oauth/callback/" + pathToken
	callback, err := startLoopbackOAuthCallback(loginCtx, "127.0.0.1", callbackPath, "")
	if err != nil {
		return fmt.Errorf("start OpenRouter sign-in callback: %w", err)
	}
	defer callback.Close()
	query := url.Values{
		"callback_url":          {callback.RedirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	if err := notify(execution.LoginNotice{URL: openRouterAuthorize + "?" + query.Encode(), Message: "Complete OpenRouter sign-in in the browser."}); err != nil {
		return err
	}
	code, err := callback.WaitBefore(loginCtx, 90*time.Second)
	if errors.Is(err, errOAuthCallbackTimeout) {
		if cfg.Bool("ui.accessible") {
			return errors.New("OpenRouter callback was not received; use the API-key login in accessible mode")
		}
		callback.Close()
		input, promptErr := promptLoginValue(loginCtx, notify, execution.LoginPrompt{
			Kind: "secret", Title: "OpenRouter authorization code", Message: "Paste the final callback URL or authorization code", Placeholder: callback.RedirectURI,
		})
		if promptErr != nil {
			return promptErr
		}
		code, err = parseOpenRouterAuthorizationInput(input)
	}
	if err != nil {
		return err
	}
	key, err := exchangeOpenRouterCode(loginCtx, code, verifier)
	if err != nil {
		return err
	}
	record := oauthRecord{Provider: "openrouter", Flow: "openrouter", AccessToken: key, TokenType: "Bearer"}
	return s.storeProviderOAuth(ctx, cfg, "openrouter", record, notify)
}

func parseOpenRouterAuthorizationInput(input string) (string, error) {
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

func exchangeOpenRouterCode(ctx context.Context, code, verifier string) (string, error) {
	body := map[string]string{"code": code, "code_verifier": verifier, "code_challenge_method": "S256"}
	var result struct {
		Key string `json:"key"`
	}
	if err := postProviderJSON(ctx, openRouterToken, body, &result); err != nil {
		return "", fmt.Errorf("OpenRouter token exchange failed: %w", err)
	}
	if result.Key == "" {
		return "", errors.New("OpenRouter token exchange returned no API key")
	}
	return result.Key, nil
}

func (s *chatSession) storeProviderOAuth(ctx context.Context, cfg config.Resolved, provider string, record oauthRecord, notify func(execution.LoginNotice) error) error {
	store, err := s.openLoginStore(ctx, cfg, cfg.String("credentials.store"), cfg.Paths.Credentials, notify)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return errors.New("cannot encode OAuth credential")
	}
	if err := store.Set(oauthReference(provider, cfg.String("credential_ref")), string(raw)); err != nil {
		return err
	}
	return deleteOptionalCredential(store, provider+"."+cfg.String("credential_ref"))
}

func deleteOptionalCredential(store credentials.Store, reference string) error {
	err := store.Delete(reference)
	if errors.Is(err, credentials.ErrNotFound) {
		return nil
	}
	return err
}

func postProviderJSON(ctx context.Context, endpoint string, body any, result any) error {
	requestBody, err := json.Marshal(body)
	if err != nil {
		return errors.New("cannot encode provider authentication request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(requestBody)))
	if err != nil {
		return errors.New("cannot create provider authentication request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 || next.URL.Scheme != "https" || next.URL.Host != request.URL.Host {
			return errors.New("authentication endpoint redirected outside its origin")
		}
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("provider authentication endpoint is unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, providerAuthBodyMax+1))
	if err != nil || len(raw) > providerAuthBodyMax {
		return errors.New("provider authentication response is invalid")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider authentication endpoint returned HTTP %d", response.StatusCode)
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return errors.New("provider authentication response is not valid JSON")
	}
	return nil
}

func newPKCEPair() (verifier, challenge string, err error) {
	verifier, err = randomURLToken(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func randomURLToken(bytesCount int) (string, error) {
	value := make([]byte, bytesCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hasForbiddenControl(value string) bool {
	for _, r := range value {
		if r == 0 || r == 10 || r == 13 || r == 27 {
			return true
		}
	}
	return false
}

var errOAuthCallbackTimeout = errors.New("OAuth callback timed out")

type oauthCallbackResult struct {
	code string
	err  error
}

type loopbackOAuthCallback struct {
	listener    net.Listener
	server      *http.Server
	results     chan oauthCallbackResult
	RedirectURI string
}

func startLoopbackOAuthCallback(ctx context.Context, redirectHost, path, expectedState string) (*loopbackOAuthCallback, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := "http://" + net.JoinHostPort(redirectHost, strconv.Itoa(port)) + path
	results := make(chan oauthCallbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(w, "Invalid callback", http.StatusMethodNotAllowed)
			return
		}
		if expectedState != "" && request.URL.Query().Get("state") != expectedState {
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}
		if request.URL.Query().Get("error") != "" {
			select {
			case results <- oauthCallbackResult{err: errors.New("provider sign-in was denied")}:
			default:
			}
			http.Error(w, "Sign-in denied", http.StatusForbidden)
			return
		}
		code := request.URL.Query().Get("code")
		if code == "" || hasForbiddenControl(code) {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}
		select {
		case results <- oauthCallbackResult{code: code}:
		default:
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "Sign-in complete. You can return to Opera.")
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	callback := &loopbackOAuthCallback{listener: listener, server: server, results: results, RedirectURI: redirectURI}
	go func() { _ = server.Serve(listener) }()
	return callback, nil
}

func (c *loopbackOAuthCallback) Wait(ctx context.Context) (string, error) {
	select {
	case result := <-c.results:
		return result.code, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (c *loopbackOAuthCallback) WaitBefore(ctx context.Context, duration time.Duration) (string, error) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case result := <-c.results:
		return result.code, result.err
	case <-timer.C:
		return "", errOAuthCallbackTimeout
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (c *loopbackOAuthCallback) Close() {
	if c == nil {
		return
	}
	_ = c.server.Close()
	_ = c.listener.Close()
}
