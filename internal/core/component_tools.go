package core

import (
	"context"
	"errors"
	"slices"

	"github.com/diasYuri/opera/internal/modules/execution"
	operaext "github.com/diasYuri/opera/internal/modules/extensions"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/policy"
	yruntime "github.com/diasYuri/y/pkg/runtime"
	"github.com/diasYuri/y/pkg/tools"
)

func (b *compositionBuilder) tools() component.Definition {
	return componentDefinition("tools", "tool-registry", []component.ServiceRef{component.Reference(yruntime.ToolRegistryKey), component.Reference(b.keys.extensions)}, []component.ServiceRef{component.Reference(b.keys.settings), component.Reference(b.keys.state)}, func(ctx context.Context, scope *component.Scope) error {
		settings, err := component.Require(scope, b.keys.settings)
		if err != nil {
			return err
		}
		state, err := component.Require(scope, b.keys.state)
		if err != nil {
			return err
		}
		cfg, opts := settings.cfg, settings.opts
		service := state.service
		records, guard := service.Records, service.Guard
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
		searchProvider, err := settings.search.create(cfg, opts)
		if err != nil {
			return err
		}
		if err := installToolPacks(ctx, registry, ToolPackContext{Config: cfg, Policy: engine, Limits: limits, Guard: guard, Questions: service.QuestionBroker, Skills: state.skills, Work: service.Work, Service: service, Search: searchProvider}, b.packs); err != nil {
			return err
		}
		extensions, err := operaext.NewWithSafePoint(ctx, cfg, registry, opts.Secret, opts.Offline, service.AtReconfigurationPoint)
		if err != nil {
			return err
		}
		if service.SessionMemory != nil && slices.ContainsFunc(b.packs, func(pack ToolPack) bool { return pack.ID == "session-context" }) {
			extensions.LegacyContextView = func(ctx context.Context, run, generation string) bool {
				metadata, err := service.Metadata(ctx, run)
				return err == nil && metadata.ToolGeneration == generation
			}
		}
		if err := scope.Track(extensions.Close); err != nil {
			return errors.Join(err, extensions.Close(context.WithoutCancel(ctx)))
		}
		if err := component.Provide(scope, b.keys.extensions, extensions); err != nil {
			return err
		}
		return component.Provide(scope, yruntime.ToolRegistryKey, registry)
	})
}
