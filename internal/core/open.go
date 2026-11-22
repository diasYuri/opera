// Package core composes public Y services using Opera-owned factories.
package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	operaext "github.com/diasYuri/opera/internal/modules/extensions"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
	yruntime "github.com/diasYuri/y/pkg/runtime"
	"github.com/diasYuri/y/pkg/search"
)

type Options struct {
	Profile           ProfileID
	Catalog           *ComponentCatalog
	Selections        map[ComponentSlot]ComponentID
	ResponseFormat    *ai.ResponseFormat
	Management        bool
	Offline           bool
	Secret            operaext.SecretLookup
	Inspection        bool
	Thinking          string
	ResumeID          string
	ContextPaths      []string
	Credential        func(context.Context, string) (string, error)
	ProviderFactory   func(context.Context, config.Resolved) (providers.Provider, error)
	SearchProvider    search.Provider
	ProviderCatalog   *providercatalog.Catalog
	ToolPacks         []ToolPack
	DisabledToolPacks []string
	verifyThinking    bool
}
type Runtime struct {
	Extensions *operaext.Manager
	Harness    *yruntime.Harness
	Service    *execution.Service
	cfg        config.Resolved
	opts       Options
}

func (r *Runtime) Close(ctx context.Context) error { return r.Harness.Close(ctx) }

func Open(ctx context.Context, cfg config.Resolved, opts Options) (*Runtime, error) {
	plan, err := ResolveComposition(cfg, opts)
	if err != nil {
		return nil, err
	}
	if err := plan.harness.Start(ctx); err != nil {
		return nil, fmt.Errorf("composition profile %q: %w", plan.ProfileID, err)
	}
	result, err := plan.runtime()
	if err != nil {
		// The caller may already be canceled. Y continues cleanup if this wait expires.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return nil, errors.Join(err, plan.harness.Close(cleanup))
	}
	return result, nil
}
