package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/diasYuri/opera/internal/adapters/credentials"
	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
)

const (
	providerLoginTimeout = 5 * time.Minute
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
	return core.EnvironmentAPIKey(ctx, provider, func(key string) string {
		value, _ := s.app.Env(key)
		return value
	})
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

func deleteOptionalCredential(store credentials.Store, reference string) error {
	err := store.Delete(reference)
	if errors.Is(err, credentials.ErrNotFound) {
		return nil
	}
	return err
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
