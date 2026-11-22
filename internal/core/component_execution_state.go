package core

import (
	"context"
	"errors"
	"os"

	"github.com/diasYuri/opera/internal/adapters/approvals/filesystem"
	"github.com/diasYuri/opera/internal/adapters/context/filesystem"
	"github.com/diasYuri/opera/internal/adapters/skills/filesystem"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/opera/internal/modules/summarizer"
	"github.com/diasYuri/opera/internal/modules/workspace"
	"github.com/diasYuri/y/pkg/component"
)

// executionState owns the product state needed while wiring tools and the
// runner. Only application publishes the assembled execution service.
type executionState struct {
	service *execution.Service
	skills  *skills.Manager
	paths   *contextPathState
}

func (b *compositionBuilder) executionState() component.Definition {
	return componentDefinition("execution-state", "execution-state",
		[]component.ServiceRef{component.Reference(b.keys.state)},
		[]component.ServiceRef{component.Reference(b.keys.settings), component.Reference(b.keys.store), component.Reference(b.keys.records)},
		func(_ context.Context, scope *component.Scope) error {
			service, settings, err := b.baseExecutionService(scope)
			if err != nil {
				return err
			}
			cfg := settings.cfg
			if !settings.opts.Management {
				if service.SessionMemory == nil {
					return errors.New("execution records must support compare-and-swap for session context")
				}
				service.Backstage = execution.NewBackstage()
			}
			paths := newContextPathState(settings.opts.ContextPaths)
			home, _ := os.UserHomeDir()
			skillManager := skills.NewManager(cfg.Paths.Workspace, home)
			service.ContextPathSource = paths
			service.Skills = skillManager
			service.Guard = &execution.Guard{ReadApprovalFile: approvalfiles.Read, Workspace: cfg.Paths.Workspace, Mode: cfg.String("policy"), Queue: workspace.NewQueue()}
			service.QuestionBroker = execution.NewQuestionBroker()
			return component.Provide(scope, b.keys.state, &executionState{service: service, skills: skillManager, paths: paths})
		})
}

func (b *compositionBuilder) baseExecutionService(scope *component.Scope) (*execution.Service, startupSettings, error) {
	settings, err := component.Require(scope, b.keys.settings)
	if err != nil {
		return nil, settings, err
	}
	store, err := component.Require(scope, b.keys.store)
	if err != nil {
		return nil, settings, err
	}
	records, err := component.Require(scope, b.keys.records)
	if err != nil {
		return nil, settings, err
	}
	cfg := settings.cfg
	service := &execution.Service{
		ReadApprovalFile: approvalfiles.Read, ResponseFormat: settings.opts.ResponseFormat,
		Store: store, Records: records, Workspace: cfg.Paths.Workspace, MaxTokens: cfg.Int("limits.context_tokens"),
		Provider: cfg.String("provider"), Model: cfg.String("model"), Mode: cfg.String("policy"),
	}
	if store, ok := records.(summarizer.Store); ok && !settings.opts.Management {
		service.SessionMemory = &summarizer.Memory{Store: store, File: contextfiles.SessionContext{Data: cfg.Paths.Data, Workspace: cfg.Paths.Workspace}}
	}
	service.Work = execution.NewWork(records, cfg.Int("agents.max_concurrent"), cfg.Int("agents.max_depth"))
	service.Work.MaxBudget = int64(cfg.Int("limits.worker_tokens")) // 0 means unlimited.
	service.Work.Service = service
	return service, settings, nil
}
