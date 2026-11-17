package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/diasYuri/opera/internal/adapters/approvals/filesystem"
	"github.com/diasYuri/opera/internal/adapters/context/filesystem"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/providers"
	yruntime "github.com/diasYuri/y/pkg/runtime"
)

func (b *compositionBuilder) application() component.Definition {
	cfg, opts := b.cfg, b.opts
	guard, skillManager, questions, contextPaths := b.guard, b.skillManager, b.questions, b.contextPaths

	return componentDefinition("application", []component.ServiceRef{component.Reference(b.keys.service)}, []component.ServiceRef{component.Reference(b.keys.runner), component.Reference(b.keys.store), component.Reference(b.keys.records), component.Reference(yruntime.ProviderKey), component.Reference(yruntime.ToolRegistryKey), component.Reference(b.keys.work), component.Reference(b.keys.extensions)}, func(_ context.Context, scope *component.Scope) error {
		runner, err := component.Require(scope, b.keys.runner)
		if err != nil {
			return err
		}
		store, err := component.Require(scope, b.keys.store)
		if err != nil {
			return err
		}
		records, err := component.Require(scope, b.keys.records)
		if err != nil {
			return err
		}
		provider, err := component.Require(scope, yruntime.ProviderKey)
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
		visionProvider, ok := provider.(providers.Provider)
		if !ok || visionProvider == nil {
			return errors.New("configured provider does not implement providers.Provider")
		}
		service := &execution.Service{ReadApprovalFile: approvalfiles.Read, ResponseFormat: opts.ResponseFormat, Records: records, Work: work, QuestionBroker: questions, ContextBudget: min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024), ContextPaths: contextPaths.Current(), ContextPathSource: contextPaths, Skills: skillManager, Runner: runner, Store: store, Guard: guard, Workspace: cfg.Paths.Workspace, Provider: cfg.String("provider"), Model: cfg.String("model"), Mode: cfg.String("policy"), MaxTokens: cfg.Int("limits.context_tokens"), LoadContext: contextfiles.Load, LoadImages: func(ctx context.Context, paths []string) ([]ai.ContentBlock, error) {
			images, err := contextfiles.Images(ctx, cfg.Paths.Workspace, paths, visionProvider.Capabilities(cfg.String("model")).Vision, min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024))
			if err == nil && len(images) != len(paths) {
				return nil, errors.New("each /image path must reference a supported image")
			}
			return images, err
		}}
		registry, err := component.Require(scope, yruntime.ToolRegistryKey)
		if err != nil {
			return err
		}
		descriptors, _ := json.Marshal(registry.List())
		service.ToolGeneration = fmt.Sprintf("%x", sha256.Sum256(append(descriptors, []byte(extensions.Digest())...)))
		extensions.SafePoint = service.CheckReconfiguration
		work.Service = service
		service.Summarizer = summarizer{provider: visionProvider, credential: opts.Credential, timeout: cfg.String("limits.process_timeout")}
		if err := scope.Track(service.Close); err != nil {
			return err
		}
		return component.Provide(scope, b.keys.service, service)
	})
}
