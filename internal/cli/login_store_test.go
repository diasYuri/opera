package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/chatgpt"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/credentials"
	"github.com/diasYuri/opera/internal/harness"
)

type fixtureKeyring struct {
	values      map[string]string
	unavailable bool
	failSet     bool
}

func (s *fixtureKeyring) Get(name string) (string, error) {
	if s.unavailable {
		return "", credentials.ErrUnavailable
	}
	v, ok := s.values[name]
	if !ok {
		return "", credentials.ErrNotFound
	}
	return v, nil
}
func (s *fixtureKeyring) Set(name, value string) error {
	if s.unavailable || s.failSet {
		return credentials.ErrUnavailable
	}
	s.values[name] = value
	return nil
}
func (s *fixtureKeyring) Delete(name string) error { delete(s.values, name); return nil }

func TestLoginRecoversFinalCredentialWriteWithoutRepeatingAuthorization(t *testing.T) {
	cfg := sessionConfig(t)
	primary := &fixtureKeyring{values: map[string]string{"openai-subscription.host": "urn:uuid:stable-host"}, failSet: true}
	a := App{OpenStore: func(kind, directory string) (credentials.Store, error) {
		if kind == "keyring" {
			return primary, nil
		}
		return credentials.Open(kind, directory)
	}}
	s := newChatSession(a, cfg, harness.Options{}, nil)
	prompts := 0
	store, err := s.openLoginStore(context.Background(), s.cfg, "keyring", cfg.Paths.Credentials, func(n app.LoginNotice) error {
		prompts++
		if n.Prompt == nil || !strings.Contains(n.Prompt.Options[1].Description, "Unencrypted") {
			t.Fatal("fallback must require an informed storage choice")
		}
		n.Prompt.Response <- "file"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := store.Get("openai-subscription.host")
	if err != nil {
		t.Fatal(err)
	}
	// This record represents the already completed authorization-code exchange.
	record := chatgpt.Record{ClientID: "oaiapp_fixture", Subject: "verified-user", AccessToken: "private-access", RefreshToken: "private-refresh", ExpiresAt: time.Now().Add(time.Hour)}
	if err := saveSubscription(store, s.cfg, record); err != nil {
		t.Fatal(err)
	}
	if prompts != 1 {
		t.Fatal("storage retry requested another authorization")
	}
	fileStore := credentials.FileStore{Directory: cfg.Paths.Credentials}
	if migrated, err := fileStore.Get("openai-subscription.host"); err != nil || migrated != host {
		t.Fatal("host identity was not preserved", err)
	}
	saved, err := readSubscription(fileStore, s.cfg)
	if err != nil || saved.AccessToken != record.AccessToken || saved.RefreshToken != record.RefreshToken {
		t.Fatal("exchanged tokens were lost", err)
	}
	if s.cfg.String("credentials.store") != "file" {
		t.Fatal("current session still uses failed keyring")
	}
	token, err := s.opts.Credential(context.Background(), chatgpt.ProviderID)
	if err != nil || token != record.AccessToken {
		t.Fatal("execution resolver still uses failed keyring", err)
	}
	if _, err := s.Command(context.Background(), "/model openai-subscription fixture"); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(config.Options{Paths: cfg.Paths})
	if err != nil || loaded.String("credentials.store") != "file" || loaded.String("model") != "fixture" {
		t.Fatal("model selection erased storage preference", err)
	}
}

func TestKeyringRecoveryBeforeAuthorizationCanRetryOrCancel(t *testing.T) {
	for _, choice := range []string{"retry", "cancel", "file"} {
		t.Run(choice, func(t *testing.T) {
			cfg := sessionConfig(t)
			primary := &fixtureKeyring{values: map[string]string{"openai-subscription.host": "existing-host"}, unavailable: true}
			fileOpens := 0
			a := App{OpenStore: func(kind, directory string) (credentials.Store, error) {
				if kind == "keyring" {
					return primary, nil
				}
				fileOpens++
				return credentials.Open(kind, directory)
			}}
			s := newChatSession(a, cfg, harness.Options{}, nil)
			store, err := s.openLoginStore(context.Background(), s.cfg, "keyring", cfg.Paths.Credentials, func(n app.LoginNotice) error {
				if choice == "retry" {
					primary.unavailable = false
				}
				n.Prompt.Response <- choice
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.Get("openai-subscription.host")
			switch choice {
			case "retry":
				if err != nil || fileOpens != 0 || s.cfg.String("credentials.store") != "keyring" {
					t.Fatal("retry unexpectedly changed storage", err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) || fileOpens != 0 || s.cfg.String("credentials.store") != "keyring" {
					t.Fatal("cancel saved credentials or changed storage", err)
				}
			case "file":
				if !errors.Is(err, credentials.ErrNotFound) || fileOpens != 1 || s.cfg.String("credentials.store") != "file" {
					t.Fatal("preflight fallback did not switch stores", err)
				}
			}
		})
	}
}
