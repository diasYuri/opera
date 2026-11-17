package core

import (
	"context"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/approvalfiles"
	"github.com/diasYuri/y/pkg/component"
)

func (b *compositionBuilder) inspection() component.Definition {
	cfg, opts := b.cfg, b.opts

	return componentDefinition("inspection", []component.ServiceRef{component.Reference(b.keys.service)}, []component.ServiceRef{component.Reference(b.keys.store), component.Reference(b.keys.records)}, func(_ context.Context, scope *component.Scope) error {
		store, err := component.Require(scope, b.keys.store)
		if err != nil {
			return err
		}
		records, err := component.Require(scope, b.keys.records)
		if err != nil {
			return err
		}
		work := app.NewWork(records, cfg.Int("agents.max_concurrent"), cfg.Int("agents.max_depth"))
		service := &app.Service{ReadApprovalFile: approvalfiles.Read, ResponseFormat: opts.ResponseFormat, Store: store, Records: records, Work: work, Workspace: cfg.Paths.Workspace, Provider: cfg.String("provider"), Model: cfg.String("model"), Mode: cfg.String("policy")}
		work.Service = service
		return component.Provide(scope, b.keys.service, service)
	})
}
