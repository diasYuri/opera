package openrouter

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/diasYuri/opera/internal/adapters/credentials"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/providers/oauth"
)

type authRecord struct {
	Provider     string       `json:"provider"`
	Flow         string       `json:"flow,omitempty"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresAt    time.Time    `json:"expires_at"`
	Config       oauth.Config `json:"config"`
}
type authenticator struct{}

func (authenticator) Supports(action string) bool { return action == "login" }

func (authenticator) Authenticate(ctx context.Context, action string, io providercatalog.AuthIO) (providercatalog.AuthResult, error) {
	if action != "login" {
		return providercatalog.AuthResult{}, errors.New("unsupported OpenRouter authentication action")
	}
	loginCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return providercatalog.AuthResult{}, errors.New("cannot prepare OpenRouter sign-in")
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	p := make([]byte, 18)
	if _, e := rand.Read(p); e != nil {
		return providercatalog.AuthResult{}, errors.New("cannot prepare OpenRouter sign-in")
	}
	path := "/oauth/callback/" + base64.RawURLEncoding.EncodeToString(p)
	if io.Callback == nil {
		return providercatalog.AuthResult{}, errors.New("browser callback is unavailable")
	}
	redirect, wait, closeFn, err := io.Callback(loginCtx, "127.0.0.1", path, "")
	if err != nil {
		return providercatalog.AuthResult{}, fmt.Errorf("start OpenRouter sign-in callback: %w", err)
	}
	defer closeFn()
	if io.Notify != nil {
		if err = io.Notify(loginCtx, providercatalog.LoginNotice{URL: AuthorizationURL(redirect, challenge), Message: "Complete OpenRouter sign-in in the browser."}); err != nil {
			return providercatalog.AuthResult{}, err
		}
	}
	code, err := wait(loginCtx, 90*time.Second)
	if errors.Is(err, context.DeadlineExceeded) {
		if io.Prompt == nil {
			return providercatalog.AuthResult{}, err
		}
		input, e := io.Prompt(loginCtx, providercatalog.LoginPrompt{Kind: "secret", Title: "OpenRouter authorization code", Message: "Paste the final callback URL or authorization code", Placeholder: redirect})
		if e != nil {
			return providercatalog.AuthResult{}, e
		}
		code, err = ParseAuthorizationInput(input)
	}
	if err != nil {
		return providercatalog.AuthResult{}, err
	}
	key, err := Exchange(loginCtx, code, verifier)
	if err != nil {
		return providercatalog.AuthResult{}, err
	}
	raw, err := json.Marshal(authRecord{Provider: "openrouter", Flow: "openrouter", AccessToken: key, TokenType: "Bearer"})
	if err != nil {
		return providercatalog.AuthResult{}, errors.New("cannot encode OAuth credential")
	}
	if err = io.Store.Set("openrouter."+io.Reference+".oauth", string(raw)); err != nil {
		return providercatalog.AuthResult{}, err
	}
	if err = io.Store.Delete("openrouter." + io.Reference); err != nil && !errors.Is(err, credentials.ErrNotFound) {
		return providercatalog.AuthResult{}, err
	}
	return providercatalog.AuthResult{Status: "stored"}, nil
}
func (authenticator) Credential(_ context.Context, io providercatalog.AuthIO) (string, error) {
	raw, err := io.Store.Get("openrouter." + io.Reference + ".oauth")
	if err != nil {
		return "", err
	}
	var rec authRecord
	if json.Unmarshal([]byte(raw), &rec) != nil || rec.Provider != "openrouter" || rec.Flow != "openrouter" || rec.AccessToken == "" {
		return "", errors.New("invalid stored OAuth credential")
	}
	return rec.AccessToken, nil
}
