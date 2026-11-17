package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"time"

	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
)

func (a App) authenticator(cfg config.Resolved, provider string) providercatalog.Authenticator {
	catalog := a.ProviderCatalog
	if catalog == nil {
		var err error
		catalog, err = core.DefaultCatalog(cfg.CustomProviders...)
		if err != nil {
			return nil
		}
	}
	return catalog.Authenticator(provider)
}
func (a App) providerAuth(action, provider string, cfg config.Resolved, jsonl bool, handler providercatalog.Authenticator) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	store, err := a.OpenStore(cfg.String("credentials.store"), cfg.Paths.Credentials)
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	io := providercatalog.AuthIO{Store: store, Reference: cfg.String("credential_ref"), CredentialDir: cfg.Paths.Credentials, Now: time.Now}
	io.Notify = func(_ context.Context, n providercatalog.LoginNotice) error {
		if jsonl {
			if a.emit("auth.authorization", map[string]string{"provider": provider, "url": n.URL}) != 0 {
				return errors.New("cannot write authorization notification")
			}
			return nil
		}
		if n.Message != "" {
			if _, err := fmt.Fprintln(a.Out, n.Message); err != nil {
				return err
			}
		}
		if n.URL != "" {
			_, err := fmt.Fprintf(a.Out, "Open this URL to authorize:\n%s\n", n.URL)
			return err
		}
		return nil
	}
	io.Callback = func(c context.Context, address, path, state string) (string, func(context.Context, time.Duration) (string, error), func(), error) {
		cb, e := startLoopbackOAuthCallback(c, address, path, state)
		if e != nil {
			return "", nil, nil, e
		}
		return cb.RedirectURI, func(wait context.Context, d time.Duration) (string, error) {
			v, e := cb.WaitBefore(wait, d)
			if errors.Is(e, errOAuthCallbackTimeout) {
				return "", context.DeadlineExceeded
			}
			return v, e
		}, cb.Close, nil
	}
	result, err := handler.Authenticate(ctx, action, io)
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	values := map[string]string{"status": result.Status, "provider": provider, "reference": cfg.String("credential_ref")}
	for k, v := range result.Values {
		values[k] = v
	}
	if jsonl {
		return a.emit("auth.status", values)
	}
	if _, err := fmt.Fprintf(a.Out, "Authentication %s for %s (reference: %s).\n", result.Status, provider, cfg.String("credential_ref")); err != nil {
		return 1
	}
	keys := make([]string, 0, len(result.Values))
	for key, value := range result.Values {
		if value != "" && key != "provider" && key != "reference" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := fmt.Fprintf(a.Out, "%s: %s\n", key, result.Values[key]); err != nil {
			return 1
		}
	}
	return 0
}
