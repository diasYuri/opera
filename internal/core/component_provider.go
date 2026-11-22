package core

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/diasYuri/opera/internal/modules/accounting"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/providers"
	yruntime "github.com/diasYuri/y/pkg/runtime"
)

func (b *compositionBuilder) provider() component.Definition {
	return componentDefinition("provider", "model-provider", []component.ServiceRef{component.Reference(yruntime.ProviderKey)}, []component.ServiceRef{component.Reference(b.keys.settings), component.Reference(b.keys.store), component.Reference(b.keys.records)}, func(ctx context.Context, scope *component.Scope) error {
		settings, err := component.Require(scope, b.keys.settings)
		if err != nil {
			return err
		}
		cfg, opts := settings.cfg, settings.opts
		if opts.Management {
			return component.Provide[agent.Provider](scope, yruntime.ProviderKey, actionProvider{})
		}
		store, err := component.Require(scope, b.keys.store)
		if err != nil {
			return err
		}
		factory := opts.ProviderFactory
		if factory == nil {
			factory = func(ctx context.Context, cfg config.Resolved) (providers.Provider, error) {
				return opts.ProviderCatalog.Create(ctx, cfg, opts.Credential)
			}
		}
		provider, err := factory(ctx, cfg)
		if err != nil {
			if provider != nil {
				err = errors.Join(err, provider.Close())
			}
			return err
		}
		if provider == nil {
			return errors.New("provider factory returned nil")
		}
		if err := scope.Track(func(context.Context) error { return provider.Close() }); err != nil {
			_ = provider.Close()
			return err
		}
		if opts.verifyThinking && opts.Thinking != "" && opts.Thinking != "off" && !provider.Capabilities(cfg.String("model")).Reasoning {
			return errors.New("selected model does not advertise reasoning support")
		}
		records, err := component.Require(scope, b.keys.records)
		if err != nil {
			return err
		}
		prices, err := accounting.LoadPrices(cfg.String("accounting.prices_file"))
		if err != nil {
			return err
		}
		tracked := &accounting.Provider{Provider: provider, Store: records, Prices: prices, Identity: func(ctx context.Context, id string) (accounting.Identity, error) {
			raw, err := store.GetMetadata(ctx, "run:"+id)
			if err != nil {
				return accounting.Identity{}, err
			}
			var m execution.Metadata
			if err := json.Unmarshal(raw, &m); err != nil {
				return accounting.Identity{}, err
			}
			return accounting.Identity{SessionID: m.SessionID, ParentID: m.ParentID}, nil
		}}
		return component.Provide[agent.Provider](scope, yruntime.ProviderKey, tracked)
	})
}
