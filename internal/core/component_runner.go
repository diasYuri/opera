package core

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/diasYuri/opera/internal/adapters/context/filesystem"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/providers"
	yruntime "github.com/diasYuri/y/pkg/runtime"
	"time"
)

func (b *compositionBuilder) runner() component.Definition {
	cfg, opts := b.cfg, b.opts
	contextPaths := b.contextPaths

	return componentDefinition("runner", []component.ServiceRef{component.Reference(b.keys.runner)}, []component.ServiceRef{component.Reference(yruntime.ProviderKey), component.Reference(yruntime.ToolRegistryKey), component.Reference(b.keys.store), component.Reference(b.keys.work), component.Reference(b.keys.extensions)}, func(_ context.Context, scope *component.Scope) error {
		provider, err := component.Require(scope, yruntime.ProviderKey)
		if err != nil {
			return err
		}
		providerAdapter, ok := provider.(providers.Provider)
		if !ok || providerAdapter == nil {
			return errors.New("configured provider does not implement providers.Provider")
		}
		registry, err := component.Require(scope, yruntime.ToolRegistryKey)
		if err != nil {
			return err
		}
		store, err := component.Require(scope, b.keys.store)
		if err != nil {
			return err
		}
		work, err := component.Require(scope, b.keys.work)
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
		options := []agent.RunnerOption{agent.WithRunnerStateStore(store), agent.WithRunnerEventStore(store), agent.WithRunnerTranscriptStore(store), agent.WithRunnerOperationStore(store), agent.WithRunnerAgentOptions(append(extensions.AgentOptions(), work.AgentOption(), agent.WithMaxTurns(64), agent.WithMaxRetries(0), agent.WithToolTimeout(timeout), agent.WithStreamDefaults(providers.StreamOptions{Timeout: timeout, MaxTokens: 4096, Reasoning: ai.ThinkingLevel(opts.Thinking), ThinkingBudgets: defaultThinkingBudgets(), ResponseFormat: opts.ResponseFormat}), agent.WithBeforeRequest(func(ctx context.Context, request *providers.StreamRequest) (*agent.HookedResponse, error) {
			if request.Options.ResponseFormat != nil {
				if err := providers.ValidateStructuredOutputRequest(*request, providerAdapter.ID(), providerAdapter.Capabilities(request.Model.ID)); err != nil {
					return nil, err
				}
			}
			if opts.Thinking != "" && opts.Thinking != "off" && !providerAdapter.Capabilities(request.Model.ID).Reasoning {
				return nil, errors.New("thinking is unsupported by the selected model")
			}
			request.Context.Messages = append([]ai.Message(nil), request.Context.Messages...)
			var imagePaths []string
			for mi := range request.Context.Messages {
				if request.Context.Messages[mi].Role != ai.RoleUser {
					continue
				}
				blocks := request.Context.Messages[mi].Content
				for bi := range blocks {
					var reference struct {
						Path string `json:"opera_image_path"`
					}
					if blocks[bi].Type == ai.ContentText && len(blocks[bi].Details) > 0 && json.Unmarshal(blocks[bi].Details, &reference) == nil && reference.Path != "" {
						imagePaths = append(imagePaths, reference.Path)
					}
				}
			}
			if len(imagePaths) > 0 {
				attached, err := contextfiles.Images(ctx, cfg.Paths.Workspace, imagePaths, providerAdapter.Capabilities(request.Model.ID).Vision, min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024))
				if err != nil {
					return nil, err
				}
				at := 0
				for mi := range request.Context.Messages {
					blocks := request.Context.Messages[mi].Content
					var resolved []ai.ContentBlock
					for _, block := range blocks {
						var reference struct {
							Path string `json:"opera_image_path"`
						}
						if block.Type == ai.ContentText && len(block.Details) > 0 && json.Unmarshal(block.Details, &reference) == nil && reference.Path != "" {
							resolved = append(resolved, attached[at])
							at++
							continue
						}
						resolved = append(resolved, block)
					}
					request.Context.Messages[mi].Content = resolved
				}
			}
			images, err := contextfiles.Images(ctx, cfg.Paths.Workspace, contextPaths.Current(), providerAdapter.Capabilities(request.Model.ID).Vision, min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024))
			if err != nil {
				return nil, err
			}
			if len(images) > 0 {
				for i := len(request.Context.Messages) - 1; i >= 0; i-- {
					if request.Context.Messages[i].Role == ai.RoleUser {
						request.Context.Messages[i].Content = append(append([]ai.ContentBlock(nil), request.Context.Messages[i].Content...), images...)
						break
					}
				}
			}
			if err := work.Charge(ctx, request.Options.RunID, providers.EstimateTokens(request.Context)+4096); err != nil {
				return nil, err
			}
			if providers.EstimateTokens(request.Context) > int64(cfg.Int("limits.context_tokens")) {
				return nil, errors.New("estimated context exceeds configured token budget")
			}
			return nil, nil
		}))...)}
		if opts.Credential != nil && !opts.Management {
			options = append(options, agent.WithRunnerCredentialResolver(CredentialResolver{Lookup: opts.Credential}))
		}
		runner := agent.NewAgentRunner(provider, registry, options...)
		if err := scope.Track(runner.Close); err != nil {
			return err
		}
		return component.Provide(scope, b.keys.runner, runner)
	})
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
