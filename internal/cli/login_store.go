package cli

import (
	"context"
	"errors"

	"github.com/diasYuri/opera/internal/adapters/credentials"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
)

// loginStore retries the same storage operation after an explicit user choice.
// In particular, a failed final Set retains the exchanged tokens in memory while
// the user chooses a store; it never starts another OAuth exchange.
type loginStore struct {
	current      credentials.Store
	openError    error
	kind         string
	cache        map[string]string
	choose       func() (string, error)
	file         func() (credentials.Store, error)
	selectedFile func() error
	reopen       func() (credentials.Store, error)
}

func (s *loginStore) run(operation func(credentials.Store) error) error {
	for {
		err := s.openError
		if err == nil {
			err = operation(s.current)
		}
		if s.kind != "keyring" || !errors.Is(err, credentials.ErrUnavailable) {
			return err
		}
		choice, err := s.choose()
		if err != nil {
			return err
		}
		switch choice {
		case "retry":
			if s.openError != nil {
				s.current, s.openError = s.reopen()
			}
			continue
		case "file":
			store, err := s.file()
			if err != nil {
				return err
			}
			for key, value := range s.cache {
				if err := store.Set(key, value); err != nil {
					return err
				}
			}
			if err := s.selectedFile(); err != nil {
				return err
			}
			s.current, s.kind, s.openError = store, "file", nil
		default:
			return context.Canceled
		}
	}
}

func (s *loginStore) Get(name string) (string, error) {
	var value string
	err := s.run(func(store credentials.Store) error { var err error; value, err = store.Get(name); return err })
	if err == nil {
		s.cache[name] = value
	}
	return value, err
}
func (s *loginStore) Set(name, value string) error {
	err := s.run(func(store credentials.Store) error { return store.Set(name, value) })
	if err == nil {
		s.cache[name] = value
	}
	return err
}
func (s *loginStore) Delete(name string) error {
	return s.run(func(store credentials.Store) error { return store.Delete(name) })
}

func chooseLoginStore(ctx context.Context, notify func(execution.LoginNotice) error) (string, error) {
	response := make(chan string, 1)
	if err := notify(execution.LoginNotice{Message: "System keyring unavailable. Choose credential storage to continue.", Prompt: &execution.LoginPrompt{
		Title: "Credential storage",
		Options: []execution.LoginOption{
			{ID: "retry", Name: "Retry system keyring", Description: "Unlock the system keyring, then try again"},
			{ID: "file", Name: "Use private local file", Description: "Unencrypted · only your OS user can access it"},
			{ID: "cancel", Name: "Cancel login", Description: "Keep the current storage setting"},
		}, Response: response,
	}}); err != nil {
		return "", err
	}
	select {
	case choice := <-response:
		return choice, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *chatSession) subscriptionLoginWithRecovery(ctx context.Context, cfg config.Resolved, notify func(execution.LoginNotice) error) error {
	a := s.app
	a.OpenStore = func(kind, directory string) (credentials.Store, error) {
		return s.openLoginStore(ctx, cfg, kind, directory, notify)
	}

	return a.interactiveSubscriptionLogin(ctx, cfg, func(u string) error {
		return notify(execution.LoginNotice{URL: u, Message: "Finish sign-in in your browser. The authorization link is above."})
	})
}

func (s *chatSession) openLoginStore(ctx context.Context, cfg config.Resolved, kind, directory string, notify func(execution.LoginNotice) error) (credentials.Store, error) {
	open := s.app.OpenStore
	store, err := open(kind, directory)
	if kind != "keyring" {
		return store, err
	}
	if err != nil && !errors.Is(err, credentials.ErrUnavailable) {
		return nil, err
	}
	return &loginStore{current: store, openError: err, kind: kind, cache: map[string]string{},
		reopen: func() (credentials.Store, error) { return open(kind, directory) },
		choose: func() (string, error) { return chooseLoginStore(ctx, notify) },
		file:   func() (credentials.Store, error) { return open("file", directory) },
		selectedFile: func() error {
			if err := config.SaveWorkspaceCredentialStore(cfg.Paths, "file"); err != nil {
				return err
			}
			s.cfg.Values["credentials.store"] = config.Value{Value: "file", Source: "interactive"}
			s.opts.Credential = s.app.credentialLookup(s.cfg)
			s.opts.Secret = s.app.extensionSecret(s.cfg)
			return nil
		},
	}, nil
}
