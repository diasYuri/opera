package harness

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/diasYuri/opera/internal/accounting"
	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/provideradapters"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/providers"
	yruntime "github.com/diasYuri/y/pkg/runtime"
)

func (b *compositionBuilder) provider() component.Definition {
	cfg, opts := b.cfg, b.opts

	return componentDefinition("provider", []component.ServiceRef{component.Reference(yruntime.ProviderKey)}, []component.ServiceRef{component.Reference(b.keys.store), component.Reference(b.keys.records)}, func(ctx context.Context, scope *component.Scope) error {
		store, err := component.Require(scope, b.keys.store)
		if err != nil {
			return err
		}
		if opts.ResumeID != "" {
			raw, err := store.GetMetadata(ctx, "run:"+opts.ResumeID)
			if err != nil {
				return err
			}
			var metadata app.Metadata
			if err := json.Unmarshal(raw, &metadata); err != nil {
				return err
			}
			if opts.ResponseFormat == nil {
				opts.ResponseFormat = metadata.ResponseFormat
			}
			if len(opts.ContextPaths) == 0 {
				opts.ContextPaths = metadata.ContextPaths
			}
			if metadata.Workspace != cfg.Paths.Workspace {
				return errors.New("checkpoint belongs to another workspace")
			}
			for key, value := range map[string]string{"provider": metadata.Provider, "model": metadata.Model} {
				if cfg.String(key) == "" {
					cfg.Values[key] = config.Value{Value: value, Source: "checkpoint"}
				}
			}
		}
		b.contextPaths.Set(opts.ContextPaths)
		if opts.Management {
			opts.Thinking = "off"
			cfg.Values["provider"] = config.Value{Value: "opera-admin", Source: "management"}
			cfg.Values["model"] = config.Value{Value: "tools", Source: "management"}
		}
		if cfg.String("provider") == "" || cfg.String("model") == "" {
			if opts.Inspection {
				return component.Provide[agent.Provider](scope, yruntime.ProviderKey, actionProvider{})
			}
			return errors.New("choose a provider and model before execution; use opera onboarding or --provider/--model")
		}
		if opts.Management {
			return component.Provide[agent.Provider](scope, yruntime.ProviderKey, actionProvider{})
		}
		factory := opts.ProviderFactory
		if factory == nil {
			catalog := opts.ProviderCatalog
			if catalog == nil {
				var err error
				catalog, err = provideradapters.DefaultCatalog(cfg.CustomProviders...)
				if err != nil {
					return err
				}
			}
			factory = func(ctx context.Context, cfg config.Resolved) (providers.Provider, error) {
				return catalog.Create(ctx, cfg, opts.Credential)
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
			var m app.Metadata
			if err := json.Unmarshal(raw, &m); err != nil {
				return accounting.Identity{}, err
			}
			return accounting.Identity{SessionID: m.SessionID, ParentID: m.ParentID}, nil
		}}
		return component.Provide[agent.Provider](scope, yruntime.ProviderKey, tracked)
	})
}
