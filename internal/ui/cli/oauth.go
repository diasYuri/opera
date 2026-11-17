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
	"github.com/diasYuri/y/pkg/providers/auth"
	"github.com/diasYuri/y/pkg/providers/oauth"
)

// This record is serialized ONLY into the separate secret store, never config,
// snapshots, events, errors or session metadata.
type oauthRecord struct {
	Provider     string       `json:"provider"`
	Flow         string       `json:"flow,omitempty"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresAt    time.Time    `json:"expires_at"`
	Config       oauth.Config `json:"config"`
}

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
	if provider != "openai" && provider != "anthropic" && provider != "google" && provider != "openai-compatible" {
		return a.fail(2, errors.New("unsupported provider"), jsonl)
	}
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
		if provider == "openai-subscription" {
			return a.subscriptionCredential(ctx, cfg)
		}
		for _, custom := range cfg.CustomProviders {
			if custom.ID() == provider {
				return core.CustomAPIKey(ctx, custom)
			}
		}
		if provider == "anthropic" {
			for _, key := range []struct {
				name   string
				bearer bool
			}{{"ANTHROPIC_AUTH_TOKEN", true}, {"ANTHROPIC_OAUTH_TOKEN", true}, {"ANTHROPIC_API_KEY", false}} {
				if value, _ := a.Env(key.name); value != "" {
					if key.bearer {
						return "Bearer " + value, nil
					}
					return value, nil
				}
			}
		}
		var envKey string
		switch provider {
		case "openrouter":
			envKey = "OPENROUTER_API_KEY"
		case "groq":
			envKey = "GROQ_API_KEY"
		case "deepseek":
			envKey = "DEEPSEEK_API_KEY"
		}
		if envKey != "" {
			if value, _ := a.Env(envKey); value != "" {
				return value, nil
			}
		}
		// An explicit process environment key is usable in headless CI without
		// accessing or unlocking a desktop keyring.
		source := auth.NewEnvSource().WithLookup(func(key string) string { value, _ := a.Env(key); return value })
		key, err := source.Resolve(ctx, provider)
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
			var record oauthRecord
			if json.Unmarshal([]byte(raw), &record) != nil || record.Provider != provider {
				return "", errors.New("invalid stored OAuth credential")
			}
			if record.Flow == "openrouter" {
				return record.AccessToken, nil
			}
			if record.Flow == "anthropic" {
				if time.Now().After(record.ExpiresAt) {
					record, err = refreshAnthropicToken(ctx, record)
					if err != nil {
						return "", err
					}
					encoded, err := json.Marshal(record)
					if err != nil {
						return "", errors.New("cannot encode refreshed credential")
					}
					if err := store.Set(oauthReference(provider, cfg.String("credential_ref")), string(encoded)); err != nil {
						return "", err
					}
				}
				return "Bearer " + record.AccessToken, nil
			}
			credential := auth.Credential{ProviderID: provider, AccessToken: record.AccessToken, RefreshToken: record.RefreshToken, TokenType: record.TokenType, ExpiresAt: record.ExpiresAt}
			if credential.Expired(time.Now()) {
				credential, err = (oauth.Client{Config: record.Config}).Refresh(ctx, credential)
				if err != nil {
					return "", err
				}
				record.AccessToken, record.RefreshToken, record.TokenType, record.ExpiresAt = credential.AccessToken, credential.RefreshToken, credential.TokenType, credential.ExpiresAt
				encoded, err := json.Marshal(record)
				if err != nil {
					return "", errors.New("cannot encode refreshed credential")
				}
				if err := store.Set(oauthReference(provider, cfg.String("credential_ref")), string(encoded)); err != nil {
					return "", err
				}
			}
			return credential.Secret(), nil
		}
		if !errors.Is(err, credentials.ErrNotFound) {
			return "", err
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
