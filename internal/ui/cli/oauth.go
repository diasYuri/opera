package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/diasYuri/opera/internal/adapters/credentials"
	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/providers/oauth"
)

// oauthRecord is serialized ONLY into the separate secret store.
type oauthRecord = core.OAuthCredentialRecord

func (a App) oauthConfig(provider string) (oauth.Config, error) {
	get := func(key string) string { v, _ := a.Env("OPERA_OAUTH_" + key); return v }
	cfg := oauth.Config{ProviderID: provider, ClientID: get("CLIENT_ID"), ClientSecret: get("CLIENT_SECRET"), AuthorizationURL: get("AUTH_URL"), TokenURL: get("TOKEN_URL"), DeviceURL: get("DEVICE_URL"), ListenAddress: get("LISTEN_ADDRESS"), Scopes: strings.Fields(get("SCOPES"))}
	if cfg.ClientID == "" {
		return cfg, errors.New("set OPERA_OAUTH_CLIENT_ID to a client registered/allowed by the provider")
	}
	if cfg.TokenURL == "" || cfg.AuthorizationURL == "" && cfg.DeviceURL == "" {
		return cfg, errors.New("configure OPERA_OAUTH_TOKEN_URL and OPERA_OAUTH_AUTH_URL (PKCE) or OPERA_OAUTH_DEVICE_URL; endpoints and client registration are provider-specific")
	}
	return cfg, nil
}

func (a App) oauthLogin(args []string, cfg config.Resolved, jsonl bool) int {
	if len(args) != 1 {
		return a.fail(2, errors.New("usage: opera auth login <provider>"), jsonl)
	}
	provider := args[0]
	oauthConfig, err := a.oauthConfig(provider)
	if err != nil {
		return a.fail(2, err, jsonl)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	client := oauth.Client{Config: oauthConfig, Notify: func(_ context.Context, notice oauth.Notice) error {
		if jsonl {
			if code := a.emit("auth.authorization", notice); code != 0 {
				return errors.New("cannot write OAuth notification")
			}
			return nil
		}
		_, err := fmt.Fprintf(a.Out, "Open this URL to authorize Opera:\n%s\nDevice code: %s\n", notice.URL, notice.UserCode)
		return err
	}}
	credential, err := client.Login(ctx)
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	store, err := a.OpenStore(cfg.String("credentials.store"), cfg.Paths.Credentials)
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	record := oauthRecord{Provider: provider, AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, TokenType: credential.TokenType, ExpiresAt: credential.ExpiresAt, Config: oauthConfig}
	raw, err := json.Marshal(record)
	if err != nil {
		return a.fail(1, errors.New("cannot encode OAuth credential"), jsonl)
	}
	if err := store.Set(oauthReference(provider, cfg.String("credential_ref")), string(raw)); err != nil {
		return a.fail(1, err, jsonl)
	}
	if jsonl {
		return a.emit("auth.status", map[string]string{"status": "stored", "kind": "oauth", "provider": provider})
	}
	return a.writeText("OAuth credential stored.\n")
}
func oauthReference(provider, ref string) string { return provider + "." + ref + ".oauth" }

func (a App) credentialLookup(cfg config.Resolved) func(context.Context, string) (string, error) {
	return func(ctx context.Context, provider string) (string, error) {
		for _, custom := range cfg.CustomProviders {
			if custom.ID() == provider {
				return core.CustomAPIKey(ctx, custom)
			}
		}
		// Provider environment conventions are resolved by composition so the
		// command-line package remains unaware of provider-specific key names.
		key, err := core.EnvironmentAPIKeyWithCatalog(ctx, provider, func(name string) string { value, _ := a.Env(name); return value }, a.ProviderCatalog)
		if err != nil || key != "" {
			return key, err
		}
		store, err := a.OpenStore(cfg.String("credentials.store"), cfg.Paths.Credentials)
		if err != nil {
			return "", err
		}
		key, err = store.Get(provider + "." + cfg.String("credential_ref"))
		if err == nil {
			return key, nil
		}
		if !errors.Is(err, credentials.ErrNotFound) {
			return "", err
		}
		raw, err := store.Get(oauthReference(provider, cfg.String("credential_ref")))
		if err == nil {
			var secret, updated string
			var changed bool
			var resolveErr error
			if handler := a.authenticator(cfg, provider); handler != nil {
				secret, resolveErr = handler.Credential(ctx, providercatalog.AuthIO{Store: store, Reference: cfg.String("credential_ref"), CredentialDir: cfg.Paths.Credentials, Now: time.Now})
			} else {
				secret, updated, changed, resolveErr = core.ResolveOAuthCredential(ctx, provider, raw, time.Now())
			}
			if resolveErr != nil {
				return "", resolveErr
			}
			if changed {
				if err := store.Set(oauthReference(provider, cfg.String("credential_ref")), updated); err != nil {
					return "", err
				}
			}
			return secret, nil
		}
		if !errors.Is(err, credentials.ErrNotFound) {
			return "", err
		}
		if handler := a.authenticator(cfg, provider); handler != nil {
			return handler.Credential(ctx, providercatalog.AuthIO{Store: store, Reference: cfg.String("credential_ref"), CredentialDir: cfg.Paths.Credentials, Now: time.Now})
		}
		key, err = store.Get(cfg.String("credential_ref"))
		if err == nil {
			return key, nil
		}
		if !errors.Is(err, credentials.ErrNotFound) {
			return "", err
		}
		return "", errors.New("no credential configured; use auth set, auth login or the provider API-key environment variable")
	}
}
