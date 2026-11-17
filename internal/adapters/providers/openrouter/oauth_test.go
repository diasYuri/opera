package openrouter

import (
	"context"
	"errors"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
	"net/url"
	"testing"
)

func TestOAuthAuthorizationAndInputContract(t *testing.T) {
	endpoint, err := url.Parse(AuthorizationURL("http://127.0.0.1/callback", "challenge"))
	if err != nil || endpoint.Host != "openrouter.ai" || endpoint.Path != "/auth" {
		t.Fatalf("unexpected endpoint: %v, %v", endpoint, err)
	}
	query := endpoint.Query()
	if query.Get("callback_url") != "http://127.0.0.1/callback" || query.Get("code_challenge") != "challenge" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization contract changed: %v", query)
	}
	for _, input := range []string{"abc", "code=abc", "http://localhost/callback?code=abc"} {
		code, err := ParseAuthorizationInput(input)
		if err != nil || code != "abc" {
			t.Fatalf("ParseAuthorizationInput(%q) = %q, %v", input, code, err)
		}
	}
	if _, err := ParseAuthorizationInput("code=bad%0Akey"); err == nil {
		t.Fatal("accepted control characters")
	}
}

type memoryCredentialStore map[string]string

func (s memoryCredentialStore) Get(k string) (string, error) {
	v, ok := s[k]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}
func (s memoryCredentialStore) Set(k, v string) error { s[k] = v; return nil }
func (s memoryCredentialStore) Delete(k string) error { delete(s, k); return nil }

func TestAuthenticatorCredentialAndUnsupportedAction(t *testing.T) {
	store := memoryCredentialStore{"openrouter.work.oauth": `{"provider":"openrouter","flow":"openrouter","access_token":"key","token_type":"Bearer","config":{}}`}
	handler := authenticator{}
	secret, err := handler.Credential(context.Background(), providercatalog.AuthIO{Store: store, Reference: "work"})
	if err != nil || secret != "key" {
		t.Fatalf("credential = %q, %v", secret, err)
	}
	if _, err := handler.Authenticate(context.Background(), "logout", providercatalog.AuthIO{}); err == nil {
		t.Fatal("unsupported authentication action was accepted")
	}
	store["openrouter.work.oauth"] = `{"provider":"other"}`
	if _, err := handler.Credential(context.Background(), providercatalog.AuthIO{Store: store, Reference: "work"}); err == nil {
		t.Fatal("invalid stored identity was accepted")
	}
}
