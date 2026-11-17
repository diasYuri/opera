package anthropic

import (
	"context"
	"errors"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOAuthAuthorizationAndInputContract(t *testing.T) {
	endpoint, err := url.Parse(AuthorizationURL("http://localhost/callback", "challenge", "state"))
	if err != nil || endpoint.Host != "claude.ai" || endpoint.Path != "/oauth/authorize" {
		t.Fatalf("unexpected endpoint: %v, %v", endpoint, err)
	}
	query := endpoint.Query()
	if query.Get("client_id") != oauthClientID || query.Get("redirect_uri") != "http://localhost/callback" || query.Get("code_challenge") != "challenge" || query.Get("state") != "state" || !strings.Contains(query.Get("scope"), "user:inference") {
		t.Fatalf("authorization contract changed: %v", query)
	}
	for _, input := range []string{"abc#state", "code=abc&state=state", "http://localhost/callback?code=abc&state=state"} {
		code, err := ParseAuthorizationInput(input, "state")
		if err != nil || code != "abc" {
			t.Fatalf("ParseAuthorizationInput(%q) = %q, %v", input, code, err)
		}
	}
	if _, err := ParseAuthorizationInput("code=abc&state=wrong", "state"); err == nil {
		t.Fatal("accepted mismatched OAuth state")
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
	store := memoryCredentialStore{"anthropic.work.oauth": `{"provider":"anthropic","flow":"anthropic","access_token":"token","refresh_token":"refresh","token_type":"Bearer","expires_at":"2999-01-01T00:00:00Z","config":{}}`}
	handler := authenticator{}
	secret, err := handler.Credential(context.Background(), providercatalog.AuthIO{Store: store, Reference: "work", Now: time.Now})
	if err != nil || secret != "Bearer token" {
		t.Fatalf("credential = %q, %v", secret, err)
	}
	if _, err := handler.Authenticate(context.Background(), "logout", providercatalog.AuthIO{}); err == nil {
		t.Fatal("unsupported authentication action was accepted")
	}
	store["anthropic.work.oauth"] = `{"provider":"other"}`
	if _, err := handler.Credential(context.Background(), providercatalog.AuthIO{Store: store, Reference: "work"}); err == nil {
		t.Fatal("invalid stored identity was accepted")
	}
}
