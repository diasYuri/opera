// Package harness composes public Y services using Opera-owned factories.
package harness

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diasYuri/opera/internal/accounting"
	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/contextfiles"
	operaext "github.com/diasYuri/opera/internal/extensions"
	"github.com/diasYuri/opera/internal/persistence"
	"github.com/diasYuri/opera/internal/skills"
	"github.com/diasYuri/opera/internal/workspace"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/policy"
	"github.com/diasYuri/y/pkg/providers"
	yruntime "github.com/diasYuri/y/pkg/runtime"
	"github.com/diasYuri/y/pkg/storage/sqlite"
	"github.com/diasYuri/y/pkg/tools"
)

type Options struct {
	ResponseFormat  *ai.ResponseFormat
	Management      bool
	Offline         bool
	Secret          operaext.SecretLookup
	Inspection      bool
	Thinking        string
	ResumeID        string
	ContextPaths    []string
	Credential      func(context.Context, string) (string, error)
	ProviderFactory func(context.Context, config.Resolved) (providers.Provider, error)
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
	recordsKey := component.NewKey[*persistence.Store]("opera.records", "1.0.0")
	storeKey := component.NewKey[*sqlite.Store]("opera.store", "1.0.0")
	runnerKey := component.NewKey[*agent.AgentRunner]("opera.runner", "1.0.0")
	serviceKey := component.NewKey[*app.Service]("opera.execution", "1.0.0")
	guard := &app.Guard{Workspace: cfg.Paths.Workspace, Mode: cfg.String("policy"), Queue: workspace.NewQueue()}
	home, _ := os.UserHomeDir()
	skillManager := skills.NewManager(cfg.Paths.Workspace, home)
	// Copy the map before adding checkpoint-derived defaults.
	values := map[string]config.Value{}
	for key, value := range cfg.Values {
		values[key] = value
	}
	cfg.Values = values
	definition := func(id string, provides, requires []component.ServiceRef, start func(context.Context, *component.Scope) error) component.Definition {
		return component.Define(component.Descriptor{ID: id, Version: "1.0.0", Provides: provides, Requires: requires}, func() (component.Component, error) { return component.ComponentFunc(start), nil })
	}
	var application *app.Service
	var work *app.Work
	var extensions *operaext.Manager
	questions := app.NewQuestionBroker()
	definitions := []component.Definition{
		definition("storage", []component.ServiceRef{component.Reference(storeKey), component.Reference(recordsKey)}, nil, func(ctx context.Context, scope *component.Scope) error {
			store, err := sqlite.Open(ctx, filepath.Join(cfg.Paths.Data, "opera.db"))
			if err != nil {
				return err
			}
			if err := scope.Track(func(context.Context) error { return store.Close() }); err != nil {
				_ = store.Close()
				return err
			}
			records, err := persistence.Open(ctx, filepath.Join(cfg.Paths.Data, "opera.db"), cfg.Paths.Workspace)
			if err != nil {
				return err
			}
			if err := scope.Track(func(context.Context) error { return records.Close() }); err != nil {
				_ = records.Close()
				return err
			}
			if err := component.Provide(scope, recordsKey, records); err != nil {
				return err
			}
			if opts.ResumeID != "" {
				raw, e := store.GetMetadata(ctx, "run:"+opts.ResumeID)
				if e != nil {
					return e
				}
				var meta app.Metadata
				if e := json.Unmarshal(raw, &meta); e != nil {
					return e
				}
				if meta.Management {
					opts.Management = true
					var action Action
					if json.Unmarshal([]byte(meta.InitialPrompt), &action) == nil {
						opts.Offline = strings.HasPrefix(action.Tool, "opera_")
					}
				}
			}
			return component.Provide(scope, storeKey, store)
		}),
		definition("provider", []component.ServiceRef{component.Reference(yruntime.ProviderKey)}, []component.ServiceRef{component.Reference(storeKey), component.Reference(recordsKey)}, func(ctx context.Context, scope *component.Scope) error {
			store, err := component.Require(scope, storeKey)
			if err != nil {
				return err
			}
			if opts.ResumeID != "" {
				raw, err := store.GetMetadata(ctx, "run:"+opts.ResumeID)
				if err != nil {
					return err
				}
				var metadata app.Metadata
				if err := json.Unmarshal(raw, &metadata); err != nil {
					return err
				}
				if opts.ResponseFormat == nil {
					opts.ResponseFormat = metadata.ResponseFormat
				}
				if len(opts.ContextPaths) == 0 {
					opts.ContextPaths = metadata.ContextPaths
				}
				if metadata.Workspace != cfg.Paths.Workspace {
					return errors.New("checkpoint belongs to another workspace")
				}
				for key, value := range map[string]string{"provider": metadata.Provider, "model": metadata.Model} {
					if cfg.String(key) == "" {
						cfg.Values[key] = config.Value{Value: value, Source: "checkpoint"}
					}
				}
			}
			if opts.Management {
				opts.Thinking = "off"
				cfg.Values["provider"] = config.Value{Value: "opera-admin", Source: "management"}
				cfg.Values["model"] = config.Value{Value: "tools", Source: "management"}
			}
			if cfg.String("provider") == "" || cfg.String("model") == "" {
				return errors.New("choose a provider and model before execution; use opera onboarding or --provider/--model")
			}
			if opts.Management {
				return component.Provide[agent.Provider](scope, yruntime.ProviderKey, actionProvider{})
			}
			factory := opts.ProviderFactory
			if factory == nil {
				factory = func(ctx context.Context, cfg config.Resolved) (providers.Provider, error) {
					return configuredProvider(ctx, cfg, opts.Credential)
				}
			}
			provider, err := factory(ctx, cfg)
			if err != nil {
				return err
			}
			if err := scope.Track(func(context.Context) error { return provider.Close() }); err != nil {
				_ = provider.Close()
				return err
			}
			records, err := component.Require(scope, recordsKey)
			if err != nil {
				return err
			}
			prices, err := accounting.LoadPrices(cfg.String("accounting.prices_file"))
			if err != nil {
				return err
			}
			tracked := &accounting.Provider{Provider: provider, Store: records, Prices: prices, Identity: func(ctx context.Context, id string) (accounting.Identity, error) {
				raw, err := store.GetMetadata(ctx, "run:"+id)
				if err != nil {
					return accounting.Identity{}, err
				}
				var m app.Metadata
				if err := json.Unmarshal(raw, &m); err != nil {
					return accounting.Identity{}, err
				}
				return accounting.Identity{SessionID: m.SessionID, ParentID: m.ParentID}, nil
			}}
			return component.Provide[agent.Provider](scope, yruntime.ProviderKey, tracked)
		}),
		definition("tools", []component.ServiceRef{component.Reference(yruntime.ToolRegistryKey)}, []component.ServiceRef{component.Reference(recordsKey)}, func(ctx context.Context, scope *component.Scope) error {
			records, err := component.Require(scope, recordsKey)
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
			if err := questions.Register(registry); err != nil {
				return err
			}
			limits := tools.ToolLimits{MaxOutputBytes: int64(cfg.Int("limits.output_bytes"))}
			registrations := []func() error{
				func() error {
					return tools.RegisterFilesystem(registry, tools.FilesystemOptions{WorkspaceRoot: cfg.Paths.Workspace, Policy: engine, Limits: limits})
				},
				func() error {
					return tools.RegisterShell(registry, tools.ShellOptions{WorkspaceRoot: cfg.Paths.Workspace, Policy: engine, Limits: limits})
				},
				func() error {
					return tools.RegisterGit(registry, tools.GitOptions{WorkspaceRoot: cfg.Paths.Workspace, Policy: engine, Limits: limits})
				},
				func() error {
					return tools.RegisterCodeSearch(registry, tools.CodeSearchOptions{WorkspaceRoot: cfg.Paths.Workspace, Policy: engine, Limits: limits})
				},
				func() error {
					return tools.RegisterLSP(registry, tools.LSPOptions{WorkspaceRoot: cfg.Paths.Workspace, Policy: engine, Limits: limits})
				},
			}
			for _, register := range registrations {
				if err := register(); err != nil {
					return err
				}
			}
			if err := skillManager.Register(registry); err != nil {
				return err
			}
			work = app.NewWork(records, cfg.Int("agents.max_concurrent"), cfg.Int("agents.max_depth"))
			work.MaxBudget = int64(cfg.Int("limits.worker_tokens"))
			if err := work.Register(registry); err != nil {
				return err
			}
			extensions, err = operaext.New(ctx, cfg, registry, opts.Secret, opts.Offline)
			if err != nil {
				return err
			}
			if err := scope.Track(extensions.Close); err != nil {
				_ = extensions.Close(context.Background())
				return err
			}
			return component.Provide(scope, yruntime.ToolRegistryKey, registry)
		}),
		definition("runner", []component.ServiceRef{component.Reference(runnerKey)}, []component.ServiceRef{component.Reference(yruntime.ProviderKey), component.Reference(yruntime.ToolRegistryKey), component.Reference(storeKey)}, func(_ context.Context, scope *component.Scope) error {
			provider, err := component.Require(scope, yruntime.ProviderKey)
			if err != nil {
				return err
			}
			registry, err := component.Require(scope, yruntime.ToolRegistryKey)
			if err != nil {
				return err
			}
			store, err := component.Require(scope, storeKey)
			if err != nil {
				return err
			}
			timeout, err := time.ParseDuration(cfg.String("limits.process_timeout"))
			if err != nil {
				return err
			}
			options := []agent.RunnerOption{agent.WithRunnerStateStore(store), agent.WithRunnerEventStore(store), agent.WithRunnerTranscriptStore(store), agent.WithRunnerOperationStore(store), agent.WithRunnerAgentOptions(append(extensions.AgentOptions(), work.AgentOption(), agent.WithMaxTurns(64), agent.WithMaxRetries(0), agent.WithToolTimeout(timeout), agent.WithStreamDefaults(providers.StreamOptions{Timeout: timeout, MaxTokens: 4096, Reasoning: ai.ThinkingLevel(opts.Thinking), ResponseFormat: opts.ResponseFormat}), agent.WithBeforeRequest(func(ctx context.Context, request *providers.StreamRequest) (*agent.HookedResponse, error) {
				if request.Options.ResponseFormat != nil {
					if err := providers.ValidateStructuredOutputRequest(*request, provider.(providers.Provider).ID(), provider.(providers.Provider).Capabilities(request.Model.ID)); err != nil {
						return nil, err
					}
				}
				if opts.Thinking != "" && opts.Thinking != "off" && !provider.(providers.Provider).Capabilities(request.Model.ID).Reasoning {
					return nil, errors.New("thinking is unsupported by the selected model")
				}
				if application != nil {
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
						attached, err := contextfiles.Images(ctx, cfg.Paths.Workspace, imagePaths, provider.(providers.Provider).Capabilities(request.Model.ID).Vision, min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024))
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
					images, err := contextfiles.Images(ctx, cfg.Paths.Workspace, application.ContextPaths, provider.(providers.Provider).Capabilities(request.Model.ID).Vision, min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024))
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
				options = append(options, agent.WithRunnerCredentialResolver(credentialResolver{opts.Credential}))
			}
			runner := agent.NewAgentRunner(provider, registry, options...)
			if err := scope.Track(runner.Close); err != nil {
				return err
			}
			return component.Provide(scope, runnerKey, runner)
		}),
		definition("application", []component.ServiceRef{component.Reference(serviceKey)}, []component.ServiceRef{component.Reference(runnerKey), component.Reference(storeKey), component.Reference(recordsKey), component.Reference(yruntime.ProviderKey), component.Reference(yruntime.ToolRegistryKey)}, func(_ context.Context, scope *component.Scope) error {
			runner, err := component.Require(scope, runnerKey)
			if err != nil {
				return err
			}
			store, err := component.Require(scope, storeKey)
			if err != nil {
				return err
			}
			records, err := component.Require(scope, recordsKey)
			if err != nil {
				return err
			}
			provider, err := component.Require(scope, yruntime.ProviderKey)
			if err != nil {
				return err
			}
			visionProvider := provider.(providers.Provider)
			service := &app.Service{ResponseFormat: opts.ResponseFormat, Records: records, Work: work, QuestionBroker: questions, ContextBudget: min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024), ContextPaths: opts.ContextPaths, Skills: skillManager, Runner: runner, Store: store, Guard: guard, Workspace: cfg.Paths.Workspace, Provider: cfg.String("provider"), Model: cfg.String("model"), Mode: cfg.String("policy"), MaxTokens: cfg.Int("limits.context_tokens"), LoadImages: func(ctx context.Context, paths []string) ([]ai.ContentBlock, error) {
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
			service.Summarizer = summarizer{provider: provider.(providers.Provider), credential: opts.Credential, timeout: cfg.String("limits.process_timeout")}
			application = service
			if err := scope.Track(service.Close); err != nil {
				return err
			}
			return component.Provide(scope, serviceKey, service)
		}),
	}
	if opts.Inspection {
		definitions = append(definitions[:2], definition("inspection", []component.ServiceRef{component.Reference(serviceKey)}, []component.ServiceRef{component.Reference(storeKey), component.Reference(recordsKey)}, func(_ context.Context, scope *component.Scope) error {
			store, err := component.Require(scope, storeKey)
			if err != nil {
				return err
			}
			records, err := component.Require(scope, recordsKey)
			if err != nil {
				return err
			}
			work := app.NewWork(records, cfg.Int("agents.max_concurrent"), cfg.Int("agents.max_depth"))
			service := &app.Service{ResponseFormat: opts.ResponseFormat, Store: store, Records: records, Work: work, Workspace: cfg.Paths.Workspace, Provider: cfg.String("provider"), Model: cfg.String("model"), Mode: cfg.String("policy")}
			work.Service = service
			return component.Provide(scope, serviceKey, service)
		}))
	}
	harness, err := yruntime.NewHarness(definitions...)
	if err != nil {
		return nil, err
	}
	if err := harness.Start(ctx); err != nil {
		return nil, err
	}
	service, err := yruntime.Resolve(harness, serviceKey)
	if err != nil {
		_ = harness.Close(context.Background())
		return nil, err
	}
	return &Runtime{Extensions: extensions, Harness: harness, Service: service, cfg: cfg, opts: opts}, nil
}
