package core

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/diasYuri/opera/internal/adapters/context/filesystem"
	contextdata "github.com/diasYuri/opera/internal/modules/context"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/providers"
	yruntime "github.com/diasYuri/y/pkg/runtime"
	"time"
)

func (b *compositionBuilder) runner() component.Definition {
	return componentDefinition("runner", "durable-runner", []component.ServiceRef{component.Reference(b.keys.runner)}, []component.ServiceRef{component.Reference(b.keys.settings), component.Reference(yruntime.ProviderKey), component.Reference(yruntime.ToolRegistryKey), component.Reference(b.keys.store), component.Reference(b.keys.state), component.Reference(b.keys.extensions)}, func(ctx context.Context, scope *component.Scope) error {
		settings, err := component.Require(scope, b.keys.settings)
		if err != nil {
			return err
		}
		cfg, opts := settings.cfg, settings.opts
		state, err := component.Require(scope, b.keys.state)
		if err != nil {
			return err
		}
		contextPaths, guard, work := state.paths, state.service.Guard, state.service.Work
		provider, err := component.Require(scope, yruntime.ProviderKey)
		if err != nil {
			return err
		}
		providerAdapter, ok := provider.(providers.Provider)
		if !ok || providerAdapter == nil {
			return errors.New("configured provider does not implement providers.Provider")
		}
		if !opts.Management {
			state.service.ModelInfo = opts.ProviderCatalog.KnownModel(cfg.String("provider"), cfg.String("model"))
		}
		registry, err := component.Require(scope, yruntime.ToolRegistryKey)
		if err != nil {
			return err
		}
		store, err := component.Require(scope, b.keys.store)
		if err != nil {
			return err
		}
		extensions, err := component.Require(scope, b.keys.extensions)
		if err != nil {
			return err
		}
		timeout, err := time.ParseDuration(cfg.String("limits.process_timeout"))
		if err != nil {
			return err
		}
		state.service.SummaryTimeout = timeout
		state.service.WaitForContext = opts.Profile == ProfileHeadless
		options := []agent.RunnerOption{agent.WithRunnerStateStore(store), agent.WithRunnerEventStore(store), agent.WithRunnerTranscriptStore(store), agent.WithRunnerOperationStore(store), agent.WithRunnerAgentOptions(append(extensions.AgentOptions(), work.AgentOption(), agent.WithMaxTurns(64), agent.WithMaxRetries(0), agent.WithToolTimeout(timeout), agent.WithStreamDefaults(providers.StreamOptions{Timeout: timeout, MaxTokens: 4096, Reasoning: ai.ThinkingLevel(opts.Thinking), ThinkingBudgets: defaultThinkingBudgets(), ResponseFormat: opts.ResponseFormat}), agent.WithBeforeRequest(func(ctx context.Context, request *providers.StreamRequest) (*agent.HookedResponse, error) {
			if request.Options.ResponseFormat != nil {
				if err := providers.ValidateStructuredOutputRequest(*request, providerAdapter.ID(), providerAdapter.Capabilities(request.Model.ID)); err != nil {
					return nil, err
				}
			}
			if opts.Thinking != "" && opts.Thinking != "off" && !providerAdapter.Capabilities(request.Model.ID).Reasoning {
				return nil, errors.New("thinking is unsupported by the selected model")
			}
			if guard != nil {
				request.Context.Tools = guard.FilterTools(request.Options.RunID, request.Context.Tools)
			}
			vision := providerAdapter.Capabilities(request.Model.ID).Vision
			limit := min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024)
			paths := contextPaths.Current()
			if err := state.service.PrepareContext(ctx, request, func(ctx context.Context, messages []ai.Message) ([]ai.Message, error) {
				return hydrateRequestImages(ctx, cfg.Paths.Workspace, messages, paths, vision, limit)
			}); err != nil {
				return nil, err
			}
			return nil, nil
		}))...)}
		if opts.Credential != nil && !opts.Management {
			options = append(options, agent.WithRunnerCredentialResolver(CredentialResolver{Lookup: opts.Credential}))
		}
		runner := agent.NewAgentRunner(provider, registry, options...)
		if err := scope.Track(runner.Close); err != nil {
			return errors.Join(err, runner.Close(context.WithoutCancel(ctx)))
		}
		return component.Provide(scope, b.keys.runner, runner)
	})
}

func hydrateRequestImages(ctx context.Context, workspace string, messages []ai.Message, paths []string, vision bool, limit int) ([]ai.Message, error) {
	var references []string
	positions := make(map[string]int)
	canonicalImage := func(block ai.ContentBlock) string {
		path := contextdata.ImageReferencePath(block)
		if path == "" {
			return ""
		}
		if !filepath.IsAbs(path) {
			return filepath.Join(workspace, path)
		}
		return filepath.Clean(path)
	}
	for _, message := range messages {
		if message.Role != ai.RoleUser {
			continue
		}
		for _, block := range message.Content {
			if path := canonicalImage(block); path != "" {
				if _, exists := positions[path]; !exists {
					positions[path] = len(references)
					references = append(references, path)
				}
			}
		}
	}

	attached, err := contextfiles.Images(ctx, workspace, references, vision, limit)
	if err != nil {
		return nil, err
	}
	if len(attached) != len(references) {
		return nil, errors.New("each /image path must reference a supported image")
	}
	images, err := contextfiles.Images(ctx, workspace, paths, vision, limit)
	if err != nil {
		return nil, err
	}
	if len(attached) == 0 && len(images) == 0 {
		return messages, nil
	}
	out := append([]ai.Message(nil), messages...)
	latestUser := -1
	for i, message := range messages {
		if message.Role != ai.RoleUser {
			continue
		}
		latestUser = i
		if len(attached) == 0 {
			continue
		}
		blocks := make([]ai.ContentBlock, 0, len(message.Content))
		for _, block := range message.Content {
			if path := canonicalImage(block); path != "" {
				block = attached[positions[path]]
			}
			blocks = append(blocks, block)
		}
		out[i].Content = blocks
	}
	if latestUser >= 0 && len(images) > 0 {
		out[latestUser].Content = append(append([]ai.ContentBlock(nil), out[latestUser].Content...), images...)
	}
	return out, nil
}

// ThinkingBudgets translates Opera's ordered levels to token budgets below
// the runner's 4096-token output limit.
func defaultThinkingBudgets() map[ai.ThinkingLevel]int64 {
	return map[ai.ThinkingLevel]int64{
		ai.ThinkingMinimal: 1024,
		ai.ThinkingLow:     1536,
		ai.ThinkingMedium:  2048,
		ai.ThinkingHigh:    2560,
		ai.ThinkingXHigh:   3072,
		ai.ThinkingMax:     3072,
	}
}
