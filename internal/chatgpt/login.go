package chatgpt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

func randomValue() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Login publishes a short-lived authorization URL; callers must not log it.
// It deliberately omits id_token_hint so terminal/JSONL output contains no tokens.
func (c *Client) Login(ctx context.Context, hostID string, previous Record, notify func(string) error) (Record, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return Record{}, errors.New("cannot start ChatGPT loopback callback listener")
	}
	redirect := "http://" + listener.Addr().String() + "/auth/callback"
	state, nonce, verifier := randomValue(), randomValue(), oauth2.GenerateVerifier()
	challenge := sha256.Sum256([]byte(verifier))
	clientID := previous.ClientID
	if clientID == "" {
		clientID = dynamicClient
	}
	q := url.Values{"client_id": {clientID}, "ext_agent_host_id": {hostID}, "response_type": {"code"}, "redirect_uri": {redirect}, "scope": {"openid profile email offline_access resource.invoke " + planScope}, "resource": {resource}, "state": {state}, "nonce": {nonce}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}}
	if clientID == dynamicClient {
		q.Set("agent_name_hint", "Opera")
	}
	type callback struct {
		code, clientID string
		err            error
	}
	result := make(chan callback, 1)
	var once sync.Once
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if r.Method != http.MethodGet || r.URL.Path != "/auth/callback" {
			http.NotFound(w, r)
			return
		}
		values := r.URL.Query()
		if values.Get("state") != state {
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}
		once.Do(func() {
			issued := values.Get("client_id")
			var err error
			switch {
			case values.Get("error") != "":
				err = errors.New("ChatGPT authorization denied or cancelled")
			case values.Get("code") == "":
				err = errors.New("ChatGPT callback missing authorization code")
			case clientID == dynamicClient && (issued == "" || issued == dynamicClient):
				err = errors.New("ChatGPT registration incomplete: missing issued client ID")
			case clientID != dynamicClient && issued != "" && issued != clientID:
				err = errors.New("ChatGPT callback changed the selected registration")
			}
			if clientID != dynamicClient {
				issued = clientID
			}
			result <- callback{values.Get("code"), issued, err}
		})
		_, _ = w.Write([]byte("Return to Opera to complete sign-in. You can close this tab."))
	})}
	defer func() { _ = server.Close() }()
	go func() { _ = server.Serve(listener) }()
	if err := notify(c.authorizeURL + "?" + q.Encode()); err != nil {
		return Record{}, err
	}
	var cb callback
	select {
	case <-ctx.Done():
		return Record{}, ctx.Err()
	case cb = <-result:
	}
	if cb.err != nil {
		return Record{}, cb.err
	}
	var tokens tokenResponse
	if err := c.post(ctx, c.tokenURL, url.Values{"grant_type": {"authorization_code"}, "client_id": {cb.clientID}, "code": {cb.code}, "code_verifier": {verifier}, "redirect_uri": {redirect}, "resource": {resource}}, &tokens); err != nil {
		return Record{}, err
	}
	subject, email, err := c.verify(ctx, tokens.IDToken, cb.clientID, nonce, previous.Subject)
	if err != nil {
		return Record{}, err
	}
	return tokens.apply(Record{ClientID: cb.clientID, Subject: subject, Email: email})
}
