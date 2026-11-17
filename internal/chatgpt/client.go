// Package chatgpt implements Sign in with ChatGPT for local, open-source clients.
package chatgpt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const ProviderID = "openai-subscription"
const issuer = "https://auth.openai.com"
const resource = "https://api.openai.com/v1"
const planScope = "chatgpt.tokens.use.direct"
const dynamicClient = "dynamic_agent_client"

// Record is stored exclusively in Opera's credential store.
type Record struct {
	ClientID     string    `json:"client_id"`
	Subject      string    `json:"subject"`
	Email        string    `json:"email,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

type Client struct {
	HTTP *http.Client
	// Endpoints are private: production never accepts endpoint overrides from config.
	issuer, tokenURL, jwksURL, authorizeURL string
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, issuer: issuer,
		tokenURL: issuer + "/api/accounts/oauth/token", jwksURL: issuer + "/.well-known/jwks.json", authorizeURL: issuer + "/api/accounts/authorize"}
}

func (c *Client) post(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("cannot build ChatGPT token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("ChatGPT token endpoint unavailable; credentials were preserved")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var body struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body)
		switch body.Error {
		case "invalid_grant", "invalid_client":
			return errors.New("ChatGPT authorization expired or revoked; run opera auth login openai-subscription again")
		case "access_denied":
			return errors.New("ChatGPT authorization denied")
		}
		return fmt.Errorf("ChatGPT token request failed (HTTP %d); credentials were preserved", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(out); err != nil {
		return errors.New("invalid ChatGPT token response")
	}
	return nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

func (t tokenResponse) apply(previous Record) (Record, error) {
	if t.AccessToken == "" || !strings.EqualFold(t.TokenType, "Bearer") || t.ExpiresIn <= 0 || t.ExpiresIn > 86400*365 {
		return Record{}, errors.New("invalid ChatGPT token response")
	}
	if t.Scope != "" {
		previous.Scope = t.Scope
	}
	if !hasScope(previous.Scope, planScope) {
		return Record{}, errors.New("ChatGPT plan usage was not authorized; enable it in ChatGPT settings and sign in again")
	}
	previous.AccessToken = t.AccessToken
	previous.ExpiresAt = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	if t.RefreshToken != "" {
		previous.RefreshToken = t.RefreshToken
	}
	if t.IDToken != "" {
		previous.IDToken = t.IDToken
	}
	return previous, nil
}

func hasScope(scopes, scope string) bool {
	for _, s := range strings.Fields(scopes) {
		if s == scope {
			return true
		}
	}
	return false
}

func (c *Client) verify(ctx context.Context, raw, clientID, nonce, subject string) (string, string, error) {
	ctx = oidc.ClientContext(ctx, c.HTTP)
	v := oidc.NewVerifier(c.issuer, oidc.NewRemoteKeySet(ctx, c.jwksURL), &oidc.Config{ClientID: clientID, SupportedSigningAlgs: []string{"RS256"}})
	id, err := v.Verify(ctx, raw)
	if err != nil || id.Subject == "" || nonce != "" && id.Nonce != nonce || subject != "" && id.Subject != subject {
		return "", "", errors.New("ChatGPT identity validation failed; saved account was not changed")
	}
	var claims struct {
		Email string `json:"email"`
	}
	if id.Claims(&claims) != nil {
		return "", "", errors.New("invalid ChatGPT identity claims")
	}
	return id.Subject, claims.Email, nil
}

func (c *Client) Refresh(ctx context.Context, record Record) (Record, error) {
	if record.RefreshToken == "" || record.ClientID == "" || record.Subject == "" {
		return Record{}, errors.New("ChatGPT session missing; run opera auth login openai-subscription")
	}
	var tokens tokenResponse
	if err := c.post(ctx, c.tokenURL, url.Values{"grant_type": {"refresh_token"}, "client_id": {record.ClientID}, "refresh_token": {record.RefreshToken}, "resource": {resource}}, &tokens); err != nil {
		return Record{}, err
	}
	if tokens.IDToken != "" {
		if _, _, err := c.verify(ctx, tokens.IDToken, record.ClientID, "", record.Subject); err != nil {
			return Record{}, err
		}
	}
	return tokens.apply(record)
}

func (c *Client) Revoke(ctx context.Context, record Record) error {
	if record.RefreshToken == "" {
		return nil
	}
	var discovery struct {
		Issuer             string `json:"issuer"`
		RevocationEndpoint string `json:"revocation_endpoint"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return errors.New("cannot build OpenAI discovery request")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("OpenAI discovery unavailable; remote revocation not confirmed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&discovery) != nil || discovery.Issuer != c.issuer {
		return errors.New("invalid OpenAI discovery response")
	}
	u, err := url.Parse(discovery.RevocationEndpoint)
	base, _ := url.Parse(c.issuer)
	if err != nil || u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil {
		return errors.New("invalid OpenAI revocation endpoint")
	}
	return c.post(ctx, u.String(), url.Values{"token": {record.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {record.ClientID}}, nil)
}
