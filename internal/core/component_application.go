package core

import (
	"context"
	"errors"

	"github.com/diasYuri/opera/internal/adapters/context/filesystem"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/providers"
	yruntime "github.com/diasYuri/y/pkg/runtime"
)

func (b *compositionBuilder) application() component.Definition {
	return componentDefinition("application", "execute", []component.ServiceRef{component.Reference(b.keys.service)}, []component.ServiceRef{component.Reference(b.keys.settings), component.Reference(b.keys.state), component.Reference(b.keys.runner), component.Reference(yruntime.ProviderKey), component.Reference(b.keys.extensions)}, func(ctx context.Context, scope *component.Scope) error {
		settings, err := component.Require(scope, b.keys.settings)
		if err != nil {
			return err
		}
		cfg, opts := settings.cfg, settings.opts
		state, err := component.Require(scope, b.keys.state)
		if err != nil {
			return err
		}
		runner, err := component.Require(scope, b.keys.runner)
		if err != nil {
			return err
		}
		provider, err := component.Require(scope, yruntime.ProviderKey)
		if err != nil {
			return err
		}
		extensions, err := component.Require(scope, b.keys.extensions)
		if err != nil {
			return err
		}
		visionProvider, ok := provider.(providers.Provider)
		if !ok || visionProvider == nil {
			return errors.New("configured provider does not implement providers.Provider")
		}
		service := state.service
		service.Runner = runner
		service.ContextBudget = min(service.MaxTokens*2, cfg.Int("limits.attachment_bytes"), 256*1024)
		service.LoadContext = contextfiles.Load
		service.LoadImages = func(ctx context.Context, paths []string) ([]ai.ContentBlock, error) {
			images, err := contextfiles.Images(ctx, cfg.Paths.Workspace, paths, visionProvider.Capabilities(cfg.String("model")).Vision, min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024))
			if err == nil && len(images) != len(paths) {
				return nil, errors.New("each /image path must reference a supported image")
			}
			return images, err
		}
		service.ToolGenerationSource = extensions.ToolGeneration
		service.LegacyToolGenerationSource = extensions.LegacyToolGeneration
		if !opts.Management {
			service.Summarizer = summaryProvider{provider: visionProvider, credential: opts.Credential, timeout: cfg.String("limits.process_timeout")}
		}
		if err := scope.Track(service.Close); err != nil {
			return errors.Join(err, service.Close(context.WithoutCancel(ctx)))
		}
		return component.Provide(scope, b.keys.service, service)
	})
}
