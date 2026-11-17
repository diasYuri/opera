package subscription

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

func TestLoginValidatesIdentityAndDynamicRegistration(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []string{"", "nonce", "issuer", "audience", "expired", "signature", "scope", "client", "denied", "subject"} {
		t.Run(failure, func(t *testing.T) {
			var authorization url.Values
			var c *Client
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/jwks" {
					_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "fixture", Algorithm: "RS256", Use: "sig"}}})
					return
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("client_id") != "oaiapp_fixture" || r.Form.Get("resource") != resource || r.Form.Get("code_verifier") == "" || r.Form.Get("redirect_uri") != authorization.Get("redirect_uri") {
					t.Error("incorrect code exchange")
				}
				claims := map[string]any{"iss": c.issuer, "sub": "account-1", "aud": "oaiapp_fixture", "nonce": authorization.Get("nonce"), "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "email": "user@example.test"}
				switch failure {
				case "nonce":
					claims["nonce"] = "wrong"
				case "issuer":
					claims["iss"] = "https://attacker.test"
				case "audience":
					claims["aud"] = "wrong"
				case "expired":
					claims["exp"] = time.Now().Add(-time.Hour).Unix()
				case "subject":
					claims["sub"] = "other-account"
				}
				signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "fixture"))
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(claims)
				signed, err := signer.Sign(raw)
				if err != nil {
					t.Fatal(err)
				}
				token, err := signed.CompactSerialize()
				if err != nil {
					t.Fatal(err)
				}
				if failure == "signature" {
					parts := strings.Split(token, ".")
					parts[2] = "invalid"
					token = strings.Join(parts, ".")
				}
				scope := planScope
				if failure == "scope" {
					scope = "openid"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "secret-access", "refresh_token": "secret-refresh", "id_token": token, "expires_in": 3600, "token_type": "Bearer", "scope": scope})
			}))
			defer server.Close()
			c = NewClient()
			c.issuer = server.URL
			c.tokenURL = server.URL + "/token"
			c.jwksURL = server.URL + "/jwks"
			c.authorizeURL = server.URL + "/authorize"
			previous := Record{}
			if failure == "subject" {
				previous = Record{ClientID: "oaiapp_fixture", Subject: "account-1"}
			}
			record, err := c.Login(context.Background(), "urn:uuid:fixture", previous, func(u string) error {
				parsed, err := url.Parse(u)
				if err != nil {
					return err
				}
				authorization = parsed.Query()
				if authorization.Get("code_challenge_method") != "S256" || authorization.Get("nonce") == "" || authorization.Get("ext_agent_host_id") != "urn:uuid:fixture" {
					t.Error("missing PKCE/nonce/host")
				}
				if previous.ClientID == "" && (authorization.Get("client_id") != dynamicClient || authorization.Get("agent_name_hint") != "Opera") {
					t.Error("missing dynamic registration")
				}
				callback, _ := url.Parse(authorization.Get("redirect_uri"))
				if callback.Hostname() != "127.0.0.1" || callback.Path != "/auth/callback" {
					t.Error("invalid loopback")
				}
				q := url.Values{"code": {"private-code"}, "state": {"wrong"}, "client_id": {"oaiapp_fixture"}}
				callback.RawQuery = q.Encode()
				response, err := http.Get(callback.String())
				if err != nil {
					return err
				}
				_ = response.Body.Close()
				if response.StatusCode != 400 {
					t.Error("invalid state accepted")
				}
				q.Set("state", authorization.Get("state"))
				if failure == "client" {
					q.Del("client_id")
				}
				if failure == "denied" {
					q.Set("error", "access_denied")
				}
				callback.RawQuery = q.Encode()
				response, err = http.Get(callback.String())
				if err == nil {
					_ = response.Body.Close()
				}
				return err
			})
			if failure == "" {
				if err != nil || record.ClientID != "oaiapp_fixture" || record.Subject != "account-1" || record.AccessToken != "secret-access" {
					t.Fatal(record.ClientID, err)
				}
			} else if err == nil {
				t.Fatal("invalid authorization accepted", failure)
			}
		})
	}
}

func TestRefreshRetainsRotationAndScopes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != "oaiapp_saved" || r.Form.Get("resource") != resource || r.Form.Get("refresh_token") != "old-refresh" || r.Form.Get("scope") != "" {
			t.Error("incorrect refresh grant")
		}
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer server.Close()
	c := NewClient()
	c.tokenURL = server.URL
	record, err := c.Refresh(context.Background(), Record{ClientID: "oaiapp_saved", Subject: "account", RefreshToken: "old-refresh", Scope: planScope})
	if err != nil || record.RefreshToken != "old-refresh" || record.AccessToken != "new-access" || record.Scope != planScope {
		t.Fatal("refresh lost credentials", err)
	}
}

func TestTokenErrorsDoNotExposeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"private-refresh"}`)
	}))
	defer server.Close()
	c := NewClient()
	c.tokenURL = server.URL
	_, err := c.Refresh(context.Background(), Record{ClientID: "saved", Subject: "account", RefreshToken: "private-refresh"})
	if err == nil || strings.Contains(err.Error(), "private-refresh") || !strings.Contains(err.Error(), "sign") && !strings.Contains(err.Error(), "login") {
		t.Fatal(err)
	}
}
