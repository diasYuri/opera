package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/diasYuri/opera/internal/adapters/providers/openai/subscription"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/credentials"
	"github.com/diasYuri/opera/internal/privatefs"
	"github.com/diasYuri/opera/internal/workspace"
	"github.com/google/uuid"
)

func subscriptionReference(cfg config.Resolved) string {
	return oauthReference(subscription.ProviderID, cfg.String("credential_ref"))
}

// The directory lock serializes token rotation across processes and workspaces,
// including when secrets themselves live in the OS keyring.
func subscriptionLock(ctx context.Context, cfg config.Resolved) (func() error, error) {
	dir := cfg.Paths.Credentials
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0700); err == nil {
		if err := privatefs.Protect(dir, true); err != nil {
			return nil, err
		}
	} else if !os.IsExist(err) {
		return nil, err
	}
	if err := privatefs.Check(dir, true); err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		unlock, err := workspace.Lock(filepath.Join(dir, "locks"), dir)
		if err == nil {
			return unlock, nil
		}
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("ChatGPT credentials are busy or cannot be locked: %w", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func readSubscription(store credentials.Store, cfg config.Resolved) (subscription.Record, error) {
	raw, err := store.Get(subscriptionReference(cfg))
	if err != nil {
		return subscription.Record{}, err
	}
	var record subscription.Record
	if json.Unmarshal([]byte(raw), &record) != nil || record.ClientID == "" || record.Subject == "" {
		return record, errors.New("invalid ChatGPT credential record")
	}
	return record, nil
}

func saveSubscription(store credentials.Store, cfg config.Resolved, record subscription.Record) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return errors.New("cannot encode ChatGPT credential")
	}
	return store.Set(subscriptionReference(cfg), string(raw))
}

func (a App) subscriptionAuth(action string, cfg config.Resolved, jsonl bool) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	unlock, err := subscriptionLock(ctx, cfg)
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	defer func() { _ = unlock() }()
	store, err := a.OpenStore(cfg.String("credentials.store"), cfg.Paths.Credentials)
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	record, err := readSubscription(store, cfg)
	if err != nil && !errors.Is(err, credentials.ErrNotFound) {
		return a.fail(1, err, jsonl)
	}
	client := subscription.NewClient()
	status := "stored"
	switch action {
	case "login":
		record, err = loginSubscription(ctx, store, cfg, record, func(u string) error {
			if jsonl {
				if a.emit("auth.authorization", map[string]string{"provider": subscription.ProviderID, "url": u}) != 0 {
					return errors.New("cannot write ChatGPT authorization notification")
				}
				return nil
			}
			_, err := fmt.Fprintf(a.Out, "Continue with ChatGPT — open this URL:\n%s\n", u)
			return err
		})
		if err != nil {
			return a.fail(1, err, jsonl)
		}

	case "logout":
		if record.ClientID != "" {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := client.Revoke(ctx, record)
			cancel()
			record.AccessToken, record.RefreshToken, record.IDToken = "", "", ""
			record.ExpiresAt = time.Time{}
			if saveErr := saveSubscription(store, cfg, record); saveErr != nil {
				return a.fail(1, saveErr, jsonl)
			}
			if err != nil {
				return a.fail(1, errors.New("signed out locally; remote revocation was not confirmed. Disconnect Opera in https://chatgpt.com/settings"), jsonl)
			}
		}
		status = "signed-out"
	case "status":
		if record.AccessToken == "" {
			status = "signed-out"
		} else if time.Now().After(record.ExpiresAt) {
			status = "expired"
		}
	default:
		return a.fail(2, errors.New("usage: opera auth <login|status|logout> openai-subscription"), jsonl)
	}
	info := map[string]string{"status": status, "provider": subscription.ProviderID, "reference": cfg.String("credential_ref"), "email": record.Email, "client_id": record.ClientID, "usage_url": "https://chatgpt.com/settings/usage"}
	if jsonl {
		return a.emit("auth.status", info)
	}
	return a.writeText(fmt.Sprintf("ChatGPT subscription: %s (reference: %s, account: %s, registration: %s).\nUsage: https://chatgpt.com/settings/usage\n", status, cfg.String("credential_ref"), record.Email, record.ClientID))
}

func (a App) subscriptionCredential(ctx context.Context, cfg config.Resolved) (string, error) {
	store, err := a.OpenStore(cfg.String("credentials.store"), cfg.Paths.Credentials)
	if err != nil {
		return "", err
	}
	record, err := readSubscription(store, cfg)
	if errors.Is(err, credentials.ErrNotFound) || err == nil && record.AccessToken == "" {
		return "", errors.New("sign in using opera auth login openai-subscription")
	}
	if err != nil {
		return "", err
	}
	if time.Now().Add(time.Minute).After(record.ExpiresAt) {
		unlock, err := subscriptionLock(ctx, cfg)
		if err != nil {
			return "", err
		}
		defer func() { _ = unlock() }()
		record, err = readSubscription(store, cfg)
		if err != nil {
			return "", err
		}
		if record.AccessToken == "" {
			return "", errors.New("ChatGPT session signed out")
		}
		if time.Now().Add(time.Minute).Before(record.ExpiresAt) {
			return record.AccessToken, nil
		}
		record, err = subscription.NewClient().Refresh(ctx, record)
		if err != nil {
			return "", err
		}
		if err := saveSubscription(store, cfg, record); err != nil {
			return "", err
		}
	}
	return record.AccessToken, nil
}

func loginSubscription(ctx context.Context, store credentials.Store, cfg config.Resolved, previous subscription.Record, notify func(string) error) (subscription.Record, error) {
	hostID, err := store.Get("openai-subscription.host")
	if errors.Is(err, credentials.ErrNotFound) {
		hostID = "urn:uuid:" + uuid.NewString()
		err = store.Set("openai-subscription.host", hostID)
	}
	if err != nil {
		return subscription.Record{}, err
	}
	record, err := subscription.NewClient().Login(ctx, hostID, previous, notify)
	if err != nil {
		return subscription.Record{}, err
	}
	if err := ctx.Err(); err != nil {
		return subscription.Record{}, err
	}
	if err := saveSubscription(store, cfg, record); err != nil {
		return subscription.Record{}, err
	}
	return record, nil
}

func (a App) interactiveSubscriptionLogin(ctx context.Context, cfg config.Resolved, notify func(string) error) error {
	unlock, err := subscriptionLock(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	store, err := a.OpenStore(cfg.String("credentials.store"), cfg.Paths.Credentials)
	if err != nil {
		return err
	}
	record, err := readSubscription(store, cfg)
	if err != nil && !errors.Is(err, credentials.ErrNotFound) {
		return err
	}
	_, err = loginSubscription(ctx, store, cfg, record, notify)
	return err
}
