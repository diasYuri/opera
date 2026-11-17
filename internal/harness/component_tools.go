package harness

import (
	"context"

	"github.com/diasYuri/opera/internal/app"
	operaext "github.com/diasYuri/opera/internal/extensions"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/policy"
	yruntime "github.com/diasYuri/y/pkg/runtime"
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
		engine := app.Policy(cfg.String("policy"), policy.AuditSinkFunc(func(ctx context.Context, event policy.AuditEvent) error {
			return records.Put(ctx, "decisions", event.EventID, event)
		}))
		registry := tools.NewRegistry(tools.WithPolicy(engine))
		if err := scope.Track(registry.Close); err != nil {
			return err
		}
		if err := registry.AddExecutionMiddleware(app.OutputLimit(cfg.Int("limits.output_bytes"), func(ctx context.Context, req tools.ToolRequest, response tools.ToolResponse) error {
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
		work := app.NewWork(records, cfg.Int("agents.max_concurrent"), cfg.Int("agents.max_depth"))
		work.MaxBudget = int64(cfg.Int("limits.worker_tokens"))
		if err := registerToolPacks(ctx, registry, ToolPackContext{Config: cfg, Policy: engine, Limits: limits, Guard: guard, Questions: questions, Skills: skillManager, Work: work}, opts.ToolPacks, opts.DisabledToolPacks); err != nil {
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
