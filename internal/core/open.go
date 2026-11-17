// Package harness composes public Y services using Opera-owned factories.
package core

import (
	"context"
	"errors"
	"os"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/approvalfiles"
	"github.com/diasYuri/opera/internal/config"
	operaext "github.com/diasYuri/opera/internal/extensions"
	"github.com/diasYuri/opera/internal/providercatalog"
	"github.com/diasYuri/opera/internal/skills"
	"github.com/diasYuri/opera/internal/workspace"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/providers"
	yruntime "github.com/diasYuri/y/pkg/runtime"
)

type Options struct {
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
	ProviderCatalog   *providercatalog.Catalog
	ToolPacks         []ToolPack
	DisabledToolPacks []string
}
type Runtime struct {
	Extensions *operaext.Manager
	Harness    *yruntime.Harness
	Service    *app.Service
	cfg        config.Resolved
	opts       Options
}

func (r *Runtime) Close(ctx context.Context) error { return r.Harness.Close(ctx) }

func Open(ctx context.Context, cfg config.Resolved, opts Options) (*Runtime, error) {
	if opts.ResponseFormat != nil {
		if err := ai.ValidateResponseFormat(*opts.ResponseFormat); err != nil {
			return nil, err
		}
	}
	switch opts.Thinking {
	case "", "off", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return nil, errors.New("unsupported thinking level")
	}
	keys := newCompositionKeys()
	guard := &app.Guard{ReadApprovalFile: approvalfiles.Read, Workspace: cfg.Paths.Workspace, Mode: cfg.String("policy"), Queue: workspace.NewQueue()}
	home, _ := os.UserHomeDir()
	skillManager := skills.NewManager(cfg.Paths.Workspace, home)
	// Copy the map before adding checkpoint-derived defaults.
	values := map[string]config.Value{}
	for key, value := range cfg.Values {
		values[key] = value
	}
	cfg.Values = values
	contextPaths := newContextPathState(opts.ContextPaths)
	questions := app.NewQuestionBroker()
	builder := &compositionBuilder{cfg: cfg, opts: &opts, keys: keys, guard: guard, skillManager: skillManager, questions: questions, contextPaths: contextPaths}
	definitions := []component.Definition{builder.storage(), builder.provider(), builder.tools(), builder.runner(), builder.application()}
	if opts.Inspection {
		definitions = append(definitions[:1], builder.inspection())
	}
	harness, err := yruntime.NewHarness(definitions...)
	if err != nil {
		return nil, err
	}
	if err := harness.Start(ctx); err != nil {
		return nil, err
	}
	service, err := yruntime.Resolve(harness, keys.service)
	if err != nil {
		_ = harness.Close(context.Background())
		return nil, err
	}
	return &Runtime{Extensions: builder.extensions, Harness: harness, Service: service, cfg: cfg, opts: opts}, nil
}
