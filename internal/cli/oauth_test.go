package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/adapters/providers/openai/subscription"
	"github.com/diasYuri/opera/internal/credentials"
)

func TestOAuthDeviceNotificationAndSeparateSecretStore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/device" {
			_, _ = fmt.Fprint(w, `{"device_code":"private-device","user_code":"VISIBLE-CODE","verification_uri":"https://example.test/authorize","expires_in":60,"interval":1}`)
		} else {
			_, _ = fmt.Fprint(w, `{"access_token":"private-access","refresh_token":"private-refresh","expires_in":600,"token_type":"Bearer"}`)
		}
	}))
	defer server.Close()
	f := newFixture(t)
	environment := map[string]string{"OPERA_OAUTH_CLIENT_ID": "fixture-client", "OPERA_OAUTH_TOKEN_URL": server.URL + "/token", "OPERA_OAUTH_DEVICE_URL": server.URL + "/device"}
	f.app.Env = func(key string) (string, bool) { value, ok := environment[key]; return value, ok }
	if code := f.app.Run([]string{"auth", "login", "openai", "--store=file", "--format=jsonl"}); code != 0 {
		t.Fatalf("OAuth login code %d: %s", code, &f.err)
	}
	if !strings.Contains(f.out.String(), "VISIBLE-CODE") {
		t.Fatal("missing device-code notification")
	}
	for _, secret := range []string{"private-device", "private-access", "private-refresh"} {
		if strings.Contains(f.out.String()+f.err.String(), secret) {
			t.Fatal("OAuth secret leaked into output")
		}
	}
	store := credentials.FileStore{Directory: filepath.Join(f.root, "data", "credentials")}
	record, err := store.Get("openai.default.oauth")
	if err != nil || !strings.Contains(record, "private-access") {
		t.Fatal("OAuth tokens were not stored separately", err)
	}
}

func TestSubscriptionAccountStatusAndLogout(t *testing.T) {
	f := newFixture(t)
	store := credentials.FileStore{Directory: filepath.Join(f.root, "data", "credentials")}
	record := subscription.Record{ClientID: "oaiapp_work", Subject: "verified-user", Email: "work@example.test", AccessToken: "private-access", IDToken: "private-identity", ExpiresAt: time.Now().Add(time.Hour)}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("openai-subscription.work.oauth", string(raw)); err != nil {
		t.Fatal(err)
	}
	if code := f.app.Run([]string{"auth", "status", "openai-subscription", "--store=file", "--set=credential_ref=work", "--format=jsonl"}); code != 0 {
		t.Fatalf("status: %d %s", code, &f.err)
	}
	if !strings.Contains(f.out.String(), "work@example.test") || !strings.Contains(f.out.String(), "oaiapp_work") {
		t.Fatal("missing selected account")
	}
	for _, secret := range []string{"private-access", "private-identity", "verified-user"} {
		if strings.Contains(f.out.String()+f.err.String(), secret) {
			t.Fatal("status leaked credential")
		}
	}
	f.out.Reset()
	if code := f.app.Run([]string{"auth", "logout", "openai-subscription", "--store=file", "--set=credential_ref=work"}); code != 0 {
		t.Fatalf("logout: %d %s", code, &f.err)
	}
	rawRecord, err := store.Get("openai-subscription.work.oauth")
	if err != nil {
		t.Fatal(err)
	}
	var signedOut subscription.Record
	if err := json.Unmarshal([]byte(rawRecord), &signedOut); err != nil {
		t.Fatal(err)
	}
	if signedOut.AccessToken != "" || signedOut.IDToken != "" || signedOut.RefreshToken != "" || signedOut.ClientID != record.ClientID || signedOut.Subject != record.Subject {
		t.Fatal("logout must clear tokens and retain registration")
	}
}

func TestSubscriptionDoesNotUseAPIKeyOrCustomEndpoint(t *testing.T) {
	f := newFixture(t)
	f.app.Env = func(key string) (string, bool) {
		if key == "OPENAI_API_KEY" {
			return "private-api-key", true
		}
		return "", false
	}
	if code := f.app.Run([]string{"models", "--provider=openai-subscription", "--store=file"}); code != 1 || !strings.Contains(f.err.String(), "auth login openai-subscription") {
		t.Fatalf("subscription fell back to API key: %d %s", code, &f.err)
	}
	f.err.Reset()
	if code := f.app.Run([]string{"models", "--provider=openai-subscription", "--base-url=https://attacker.test"}); code != 1 || !strings.Contains(f.err.String(), "does not accept base_url") {
		t.Fatalf("custom endpoint accepted: %d %s", code, &f.err)
	}
}

func TestFirstRunAccessibleChatOffersLoginWithoutOnboarding(t *testing.T) {
	f := newFixture(t)
	f.app.IsTTY = true
	f.app.In = strings.NewReader("/login\n/cancel\n/quit\n")
	if code := f.app.Run([]string{"chat", "--accessible", "--no-motion"}); code != 0 {
		t.Fatalf("first run chat: %d %s", code, &f.err)
	}
	if !strings.Contains(f.out.String(), "Connect a provider") || !strings.Contains(f.out.String(), "1. OpenAI") || strings.Contains(f.out.String(), "Anthropic") {
		t.Fatal("first-run login submenu missing", f.out.String())
	}
}
