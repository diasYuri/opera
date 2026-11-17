package anthropic

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
		return providercatalog.AuthResult{}, errors.New("unsupported Anthropic authentication action")
	}
	method := "browser"
	if !io.Accessible && io.Prompt != nil {
		choice, err := io.Prompt(ctx, providercatalog.LoginPrompt{Kind: "select", Title: "Claude sign-in", Message: "Choose a Claude account login method.", Options: []providercatalog.LoginOption{{ID: "browser", Name: "Browser login", Description: "Local callback on this machine"}, {ID: "copy-code", Name: "Copy code", Description: "Paste the code from the browser"}}})
		if err != nil {
			return providercatalog.AuthResult{}, err
		}
		method = choice
	}
	loginCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return providercatalog.AuthResult{}, errors.New("cannot prepare Anthropic sign-in")
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	redirect := CodeReturnURL()
	var wait func(context.Context, time.Duration) (string, error)
	var closeFn func()
	if method == "browser" {
		if io.Callback == nil {
			return providercatalog.AuthResult{}, errors.New("browser callback is unavailable")
		}
		var err error
		redirect, wait, closeFn, err = io.Callback(loginCtx, "localhost", "/callback", verifier)
		if err != nil {
			return providercatalog.AuthResult{}, fmt.Errorf("start Anthropic sign-in callback: %w", err)
		}
		defer closeFn()
	} else if method != "copy-code" {
		return providercatalog.AuthResult{}, errors.New("unknown Anthropic sign-in method")
	}
	if io.Notify != nil {
		if err := io.Notify(loginCtx, providercatalog.LoginNotice{URL: AuthorizationURL(redirect, challenge, verifier), Message: "Complete Anthropic sign-in in the browser."}); err != nil {
			return providercatalog.AuthResult{}, err
		}
	}
	var code string
	var err error
	if method == "browser" {
		code, err = wait(loginCtx, 5*time.Minute)
	} else {
		var input string
		input, err = io.Prompt(loginCtx, providercatalog.LoginPrompt{Kind: "secret", Title: "Claude authorization code", Message: "Paste the code or redirect URL shown after signing in", Placeholder: "authorization code"})
		if err == nil {
			code, err = ParseAuthorizationInput(input, verifier)
		}
	}
	if err != nil {
		return providercatalog.AuthResult{}, err
	}
	token, err := Exchange(loginCtx, code, verifier, redirect)
	if err != nil {
		return providercatalog.AuthResult{}, err
	}
	raw, err := json.Marshal(authRecord{Provider: "anthropic", Flow: "anthropic", AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: "Bearer", ExpiresAt: token.ExpiresAt})
	if err != nil {
		return providercatalog.AuthResult{}, errors.New("cannot encode OAuth credential")
	}
	if err = io.Store.Set("anthropic."+io.Reference+".oauth", string(raw)); err != nil {
		return providercatalog.AuthResult{}, err
	}
	if err = io.Store.Delete("anthropic." + io.Reference); err != nil && !errors.Is(err, credentials.ErrNotFound) {
		return providercatalog.AuthResult{}, err
	}
	return providercatalog.AuthResult{Status: "stored"}, nil
}
func (authenticator) Credential(ctx context.Context, io providercatalog.AuthIO) (string, error) {
	raw, err := io.Store.Get("anthropic." + io.Reference + ".oauth")
	if err != nil {
		return "", err
	}
	var rec authRecord
	if json.Unmarshal([]byte(raw), &rec) != nil || rec.Provider != "anthropic" || rec.Flow != "anthropic" {
		return "", errors.New("invalid stored OAuth credential")
	}
	if io.Now != nil && io.Now().After(rec.ExpiresAt) {
		token, e := Refresh(ctx, rec.RefreshToken)
		if e != nil {
			return "", e
		}
		rec.AccessToken, rec.RefreshToken, rec.ExpiresAt = token.AccessToken, token.RefreshToken, token.ExpiresAt
		updated, e := json.Marshal(rec)
		if e != nil {
			return "", errors.New("cannot encode refreshed credential")
		}
		if e = io.Store.Set("anthropic."+io.Reference+".oauth", string(updated)); e != nil {
			return "", e
		}
	}
	return "Bearer " + rec.AccessToken, nil
}
