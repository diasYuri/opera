// Package providercatalog owns explicit, instance-scoped provider factories.
package providercatalog

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/y/pkg/providers"
)

type Factory func(context.Context, config.Resolved, func(context.Context, string) (string, error)) (providers.Provider, error)

// LoginMethod describes a credential flow that selects a provider in this catalog.
type LoginMethod struct {
	ID          string
	Name        string
	Description string
	Kind        string
	ProviderID  string
}

// LoginDescriptor groups credential methods into one user-facing provider choice.
type LoginDescriptor struct {
	ID          string
	Name        string
	Description string
	Methods     []LoginMethod
}

type Entry struct {
	ID          string
	Name        string
	Description string
	Factory     Factory
	Login       *LoginDescriptor
}

type Catalog struct{ entries map[string]Entry }

func New(entries ...Entry) (*Catalog, error) {
	items := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		if entry.ID == "" || entry.Factory == nil {
			return nil, errors.New("provider catalog entries require ID and factory")
		}
		if _, exists := items[entry.ID]; exists {
			return nil, fmt.Errorf("duplicate provider %q", entry.ID)
		}
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
		entry.Login = cloneLoginDescriptor(entry.Login)
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
