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

type Entry struct {
	ID          string
	Name        string
	Description string
	Factory     Factory
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
		return nil, fmt.Errorf("create provider %q: %w", id, err)
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %q factory returned nil", id)
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
		out = append(out, entry)
	}
	return out
}
