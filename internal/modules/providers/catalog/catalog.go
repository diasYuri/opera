// Package providercatalog owns explicit, instance-scoped provider factories.
package providercatalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/search"
)

type Factory func(context.Context, config.Resolved, func(context.Context, string) (string, error)) (providers.Provider, error)
type SearchFactory func(config.Resolved, func(context.Context, string) (string, error)) search.Provider

// CredentialStore and AuthIO expose only effects needed by provider-owned
// authentication handlers. UI supplies prompts and notifications.
type CredentialStore interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}
type LoginOption struct{ ID, Name, Description string }
type LoginPrompt struct {
	Kind, Title, Message, Placeholder string
	Options                           []LoginOption
}
type LoginNotice struct{ URL, Message string }
type AuthIO struct {
	Store         CredentialStore
	Reference     string
	CredentialDir string
	Accessible    bool
	Now           func() time.Time
	Prompt        func(context.Context, LoginPrompt) (string, error)
	Notify        func(context.Context, LoginNotice) error
	Callback      func(context.Context, string, string, string) (string, func(context.Context, time.Duration) (string, error), func(), error)
}
type AuthResult struct {
	Status string
	Values map[string]string
}
type Authenticator interface {
	Supports(string) bool
	Authenticate(context.Context, string, AuthIO) (AuthResult, error)
	Credential(context.Context, AuthIO) (string, error)
}

// LoginMethod describes a credential flow that selects a provider in this catalog.
type LoginMethod struct {
	ID            string
	Name          string
	Description   string
	Kind          string
	ProviderID    string
	Authenticator Authenticator
}

// LoginDescriptor groups credential methods into one user-facing provider choice.
type LoginDescriptor struct {
	ID          string
	Name        string
	Description string
	Methods     []LoginMethod
}

type Entry struct {
	ID                    string
	Name                  string
	Description           string
	Factory               Factory
	SearchFactory         SearchFactory
	SearchAvailable       func(config.Resolved) bool
	SearchName            string
	SearchDescription     string
	Login                 *LoginDescriptor
	EnvironmentCredential func(func(string) string) string
	FixedEndpoint         bool
}

type Catalog struct{ entries map[string]Entry }

func (c *Catalog) Authenticator(provider string) Authenticator {
	if c == nil {
		return nil
	}
	for _, id := range c.IDs() {
		entry := c.entries[id]
		if entry.Login == nil {
			continue
		}
		for _, method := range entry.Login.Methods {
			if method.ProviderID == provider && method.Authenticator != nil {
				return method.Authenticator
			}
		}
	}
	return nil
}

func (c *Catalog) EnvironmentCredential(provider string, lookup func(string) string) string {
	if c == nil {
		return ""
	}
	entry, ok := c.entries[provider]
	if !ok || entry.EnvironmentCredential == nil {
		return ""
	}
	return entry.EnvironmentCredential(lookup)
}

func (c *Catalog) FixedEndpoint(provider string) bool {
	if c == nil {
		return false
	}
	entry, ok := c.entries[provider]
	return ok && entry.FixedEndpoint
}

func New(entries ...Entry) (*Catalog, error) {
	items := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		if entry.ID == "" || entry.Factory == nil {
			return nil, errors.New("provider catalog entries require ID and factory")
		}
		if _, exists := items[entry.ID]; exists {
			return nil, fmt.Errorf("duplicate provider %q", entry.ID)
		}
		entry.Login = cloneLoginDescriptor(entry.Login)
		items[entry.ID] = entry
	}
	for _, entry := range items {
		if entry.Login == nil {
			continue
		}
		login := entry.Login
		if login.ID == "" || login.Name == "" || len(login.Methods) == 0 {
			return nil, fmt.Errorf("provider %q has an incomplete login descriptor", entry.ID)
		}
		seen := make(map[string]struct{}, len(login.Methods))
		for _, method := range login.Methods {
			if method.ID == "" || method.Name == "" || method.Kind == "" || method.ProviderID == "" {
				return nil, fmt.Errorf("provider %q has an incomplete login method", entry.ID)
			}
			if _, exists := seen[method.ID]; exists {
				return nil, fmt.Errorf("provider %q has duplicate login method %q", entry.ID, method.ID)
			}
			seen[method.ID] = struct{}{}
			if _, exists := items[method.ProviderID]; !exists {
				return nil, fmt.Errorf("provider %q login method %q targets unregistered provider %q", entry.ID, method.ID, method.ProviderID)
			}
		}
	}
	return &Catalog{entries: items}, nil
}

func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	ids := make([]string, 0, len(c.entries))
	for id := range c.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (c *Catalog) Create(ctx context.Context, cfg config.Resolved, lookup func(context.Context, string) (string, error)) (providers.Provider, error) {
	if c == nil {
		return nil, errors.New("provider catalog is unavailable")
	}
	id := cfg.String("provider")
	entry, ok := c.entries[id]
	if !ok {
		return nil, fmt.Errorf("provider %q is not registered", id)
	}
	provider, err := entry.Factory(ctx, cfg, lookup)
	if err != nil {
		if provider != nil {
			err = errors.Join(err, provider.Close())
		}
		return nil, fmt.Errorf("create provider %q: %w", id, err)
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %q factory returned nil", id)
	}
	if provider.ID() != id {
		mismatch := fmt.Errorf("provider %q factory returned provider %q", id, provider.ID())
		return nil, errors.Join(mismatch, provider.Close())
	}
	return provider, nil
}

func (c *Catalog) Has(id string) bool {
	if c == nil {
		return false
	}
	_, ok := c.entries[id]
	return ok
}

type SearchEntry struct {
	ID, Name, Description string
}

// SearchEntries lists the native search adapters that this catalog can
// construct. Availability in one configuration is checked separately.
func (c *Catalog) SearchEntries() []SearchEntry {
	var result []SearchEntry
	for _, id := range c.IDs() {
		entry := c.entries[id]
		if entry.SearchFactory == nil {
			continue
		}
		name := entry.SearchName
		if name == "" {
			name = entry.Name
		}
		result = append(result, SearchEntry{ID: id, Name: name, Description: entry.SearchDescription})
	}
	return result
}

func (c *Catalog) HasNativeSearch(id string, cfg config.Resolved) bool {
	if c == nil {
		return false
	}
	entry := c.entries[id]
	return entry.SearchFactory != nil && (entry.SearchAvailable == nil || entry.SearchAvailable(cfg))
}

// SearchProvider returns a provider-owned web search capability for the
// selected model provider. Authentication is resolved when a search executes.
func (c *Catalog) SearchProvider(provider string, cfg config.Resolved, lookup func(context.Context, string) (string, error)) search.Provider {
	if !c.HasNativeSearch(provider, cfg) {
		return nil
	}
	return c.entries[provider].SearchFactory(cfg, lookup)
}

// Entries returns provider selection metadata in stable ID order.
func (c *Catalog) Entries() []Entry {
	if c == nil {
		return nil
	}
	ids := c.IDs()
	out := make([]Entry, 0, len(ids))
	for _, id := range ids {
		entry := c.entries[id]
		entry.Factory = nil
		entry.SearchFactory = nil
		entry.SearchAvailable = nil
		entry.EnvironmentCredential = nil
		entry.Login = cloneLoginDescriptor(entry.Login)
		if entry.Login != nil {
			for i := range entry.Login.Methods {
				entry.Login.Methods[i].Authenticator = nil
			}
		}
		out = append(out, entry)
	}
	return out
}

// Logins returns the unique provider login groups in stable ID order.
func (c *Catalog) Logins() []LoginDescriptor {
	if c == nil {
		return nil
	}
	groups := make(map[string]LoginDescriptor)
	for _, entry := range c.entries {
		if entry.Login == nil {
			continue
		}
		if _, exists := groups[entry.Login.ID]; !exists {
			groups[entry.Login.ID] = *cloneLoginDescriptor(entry.Login)
		}
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]LoginDescriptor, 0, len(ids))
	for _, id := range ids {
		out = append(out, groups[id])
	}
	return out
}

func cloneLoginDescriptor(value *LoginDescriptor) *LoginDescriptor {
	if value == nil {
		return nil
	}
	clone := *value
	clone.Methods = append([]LoginMethod(nil), value.Methods...)
	return &clone
}
