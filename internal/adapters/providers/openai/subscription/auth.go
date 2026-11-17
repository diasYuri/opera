package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diasYuri/opera/internal/adapters/credentials"
	"github.com/diasYuri/opera/internal/adapters/platform"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/opera/internal/modules/workspace"
	"github.com/google/uuid"
)

type authenticator struct{ client *Client }

func (authenticator) Supports(action string) bool {
	return action == "login" || action == "status" || action == "logout"
}

func newAuthenticator() authenticator { return authenticator{client: NewClient()} }
func (a authenticator) reference(io providercatalog.AuthIO) string {
	return ProviderID + "." + io.Reference + ".oauth"
}
func (a authenticator) read(io providercatalog.AuthIO) (Record, error) {
	raw, err := io.Store.Get(a.reference(io))
	if err != nil {
		return Record{}, err
	}
	var r Record
	if json.Unmarshal([]byte(raw), &r) != nil || r.ClientID == "" || r.Subject == "" {
		return r, errors.New("invalid ChatGPT credential record")
	}
	return r, nil
}
func (a authenticator) save(io providercatalog.AuthIO, r Record) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return errors.New("cannot encode ChatGPT credential")
	}
	return io.Store.Set(a.reference(io), string(raw))
}
func (a authenticator) lock(ctx context.Context, io providercatalog.AuthIO) (func() error, error) {
	dir := io.CredentialDir
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0700); err == nil {
		if err = platform.Protect(dir, true); err != nil {
			return nil, err
		}
	} else if !os.IsExist(err) {
		return nil, err
	}
	if err := platform.Check(dir, true); err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var last error
	for {
		unlock, err := workspace.Lock(filepath.Join(dir, "locks"), dir)
		if err == nil {
			return unlock, nil
		}
		last = err
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("ChatGPT credentials are busy or cannot be locked: %w", last)
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func (a authenticator) Authenticate(ctx context.Context, action string, io providercatalog.AuthIO) (providercatalog.AuthResult, error) {
	unlock, err := a.lock(ctx, io)
	if err != nil {
		return providercatalog.AuthResult{}, err
	}
	defer func() { _ = unlock() }()
	record, err := a.read(io)
	if err != nil && !errors.Is(err, credentials.ErrNotFound) {
		return providercatalog.AuthResult{}, err
	}
	if errors.Is(err, credentials.ErrNotFound) {
		record = Record{}
	}
	now := time.Now()
	if io.Now != nil {
		now = io.Now()
	}
	status := "stored"
	switch action {
	case "login":
		hostID, e := io.Store.Get(ProviderID + ".host")
		if errors.Is(e, credentials.ErrNotFound) {
			hostID = "urn:uuid:" + uuid.NewString()
			e = io.Store.Set(ProviderID+".host", hostID)
		}
		if e != nil {
			return providercatalog.AuthResult{}, e
		}
		if io.Notify == nil {
			return providercatalog.AuthResult{}, errors.New("ChatGPT authorization notification unavailable")
		}
		record, err = a.client.Login(ctx, hostID, record, func(u string) error {
			return io.Notify(ctx, providercatalog.LoginNotice{URL: u, Message: "Finish sign-in in your browser."})
		})
		if err != nil {
			return providercatalog.AuthResult{}, err
		}
		if err = ctx.Err(); err != nil {
			return providercatalog.AuthResult{}, err
		}
		if err = a.save(io, record); err != nil {
			return providercatalog.AuthResult{}, err
		}
	case "logout":
		if record.ClientID != "" {
			revokeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			remoteErr := a.client.Revoke(revokeCtx, record)
			cancel()
			record.AccessToken, record.RefreshToken, record.IDToken = "", "", ""
			record.ExpiresAt = time.Time{}
			if err = a.save(io, record); err != nil {
				return providercatalog.AuthResult{}, err
			}
			if remoteErr != nil {
				return providercatalog.AuthResult{}, errors.New("signed out locally; remote revocation was not confirmed. Disconnect Opera in https://chatgpt.com/settings")
			}
		}
		status = "signed-out"
	case "status":
		if record.AccessToken == "" {
			status = "signed-out"
		} else if now.After(record.ExpiresAt) {
			status = "expired"
		}
	default:
		return providercatalog.AuthResult{}, errors.New("usage: opera auth <login|status|logout> openai-subscription")
	}
	return providercatalog.AuthResult{Status: status, Values: map[string]string{"provider": ProviderID, "reference": io.Reference, "email": record.Email, "client_id": record.ClientID, "usage_url": "https://chatgpt.com/settings/usage"}}, nil
}
func (a authenticator) Credential(ctx context.Context, io providercatalog.AuthIO) (string, error) {
	record, err := a.read(io)
	if errors.Is(err, credentials.ErrNotFound) {
		return "", errors.New("sign in using opera auth login openai-subscription")
	}
	if err != nil {
		return "", err
	}
	if record.AccessToken == "" {
		return "", errors.New("sign in using opera auth login openai-subscription")
	}
	now := time.Now()
	if io.Now != nil {
		now = io.Now()
	}
	if now.Add(time.Minute).After(record.ExpiresAt) {
		unlock, e := a.lock(ctx, io)
		if e != nil {
			return "", e
		}
		defer func() { _ = unlock() }()
		record, err = a.read(io)
		if err != nil {
			return "", err
		}
		if record.AccessToken == "" {
			return "", errors.New("ChatGPT session signed out")
		}
		if now.Add(time.Minute).Before(record.ExpiresAt) {
			return record.AccessToken, nil
		}
		record, err = a.client.Refresh(ctx, record)
		if err != nil {
			return "", err
		}
		if err = a.save(io, record); err != nil {
			return "", err
		}
	}
	return record.AccessToken, nil
}
func Authenticator() providercatalog.Authenticator { return newAuthenticator() }
