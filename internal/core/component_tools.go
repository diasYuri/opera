package core

import (
	"context"
	"errors"
	"fmt"

	searchadapter "github.com/diasYuri/opera/internal/adapters/search"
	bravesearch "github.com/diasYuri/opera/internal/adapters/search/brave"
	"github.com/diasYuri/opera/internal/modules/execution"
	operaext "github.com/diasYuri/opera/internal/modules/extensions"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/policy"
	yruntime "github.com/diasYuri/y/pkg/runtime"
	searchcontract "github.com/diasYuri/y/pkg/search"
	"github.com/diasYuri/y/pkg/tools"
)

func (b *compositionBuilder) tools() component.Definition {
	cfg, opts := b.cfg, b.opts
	guard, skillManager, questions := b.guard, b.skillManager, b.questions

	return componentDefinition("tools", []component.ServiceRef{component.Reference(yruntime.ToolRegistryKey), component.Reference(b.keys.work), component.Reference(b.keys.extensions)}, []component.ServiceRef{component.Reference(b.keys.records)}, func(ctx context.Context, scope *component.Scope) error {
		records, err := component.Require(scope, b.keys.records)
		if err != nil {
			return err
		}
		engine := execution.Policy(cfg.String("policy"), policy.AuditSinkFunc(func(ctx context.Context, event policy.AuditEvent) error {
			return records.Put(ctx, "decisions", event.EventID, event)
		}))
		registry := tools.NewRegistry(tools.WithPolicy(engine))
		if err := scope.Track(registry.Close); err != nil {
			return err
		}
		if err := registry.AddExecutionMiddleware(execution.OutputLimit(cfg.Int("limits.output_bytes"), func(ctx context.Context, req tools.ToolRequest, response tools.ToolResponse) error {
			return records.Put(context.WithoutCancel(ctx), "tool-output", req.RunID+":"+req.ID, struct {
				RunID, ToolCallID, Tool string
				Response                tools.ToolResponse
			}{req.RunID, req.ID, req.Name, response})
		})); err != nil {
			return err
		}
		if err := registry.AddExecutionMiddleware(guard.Middleware); err != nil {
			return err
		}
		limits := tools.ToolLimits{MaxOutputBytes: int64(cfg.Int("limits.output_bytes"))}
		work := execution.NewWork(records, cfg.Int("agents.max_concurrent"), cfg.Int("agents.max_depth"))
		work.MaxBudget = int64(cfg.Int("limits.worker_tokens")) // 0 means unlimited.
		searchProvider := opts.SearchProvider
		searchConfigProviderID := ""
		if searchProvider == nil {
			selection := cfg.String("search.provider")
			switch selection {
			case "":
				// A project may choose a model provider, but it cannot thereby
				// activate an external network capability. Users can explicitly
				// opt in with search.provider="native" in user settings.
				if cfg.Values["provider"].Source == "project" && cfg.Values["search.provider"].Source == "default" {
					break
				}
				fallthrough
			case "native":
				catalog := opts.ProviderCatalog
				if catalog == nil {
					catalog, err = DefaultCatalog(cfg.CustomProviders...)
					if err != nil {
						return err
					}
				}
				searchProvider = catalog.SearchProvider(cfg.String("provider"), cfg, opts.Credential)
				searchConfigProviderID = cfg.String("provider")
				if selection == "native" && searchProvider == nil {
					return fmt.Errorf("native web search is not available for provider %q", cfg.String("provider"))
				}
			case "disabled":
			case "brave":
				searchConfigProviderID = "brave"
				if opts.Credential == nil {
					return errors.New("search provider Brave Search is configured but no credential resolver is available")
				}
				searchProvider = bravesearch.NewWithCredentialResolver(func(ctx context.Context) (string, error) {
					key, err := opts.Credential(ctx, "brave-search")
					if err != nil {
						return "", fmt.Errorf("resolve Brave Search credential: %w", err)
					}
					return key, nil
				})
			default:
				return fmt.Errorf("unsupported search provider %q", cfg.String("search.provider"))
			}
		}
		if searchProvider != nil && searchConfigProviderID != "" {
			defaults := cfg.WebSearchDefaults(searchConfigProviderID)
			searchProvider = searchadapter.WithDefaults(searchProvider, searchcontract.Request{
				AllowedDomains: defaults.AllowedDomains, ExcludedDomains: defaults.ExcludedDomains,
				ContextSize: searchcontract.ContextSize(defaults.ContextSize), MaxResults: defaults.MaxResults,
				Country: defaults.Country, Language: defaults.Language, SafeSearch: searchcontract.SafeSearch(defaults.SafeSearch),
			})
		}
		if err := registerToolPacks(ctx, registry, ToolPackContext{Config: cfg, Policy: engine, Limits: limits, Guard: guard, Questions: questions, Skills: skillManager, Work: work, Search: searchProvider}, opts.ToolPacks, opts.DisabledToolPacks); err != nil {
			return err
		}
		b.extensions, err = operaext.New(ctx, cfg, registry, opts.Secret, opts.Offline)
		if err != nil {
			return err
		}
		if err := scope.Track(b.extensions.Close); err != nil {
			_ = b.extensions.Close(context.Background())
			return err
		}
		if err := component.Provide(scope, b.keys.work, work); err != nil {
			return err
		}
		if err := component.Provide(scope, b.keys.extensions, b.extensions); err != nil {
			return err
		}
		return component.Provide(scope, yruntime.ToolRegistryKey, registry)
	})
}
