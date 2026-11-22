package extensions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
	extensionhost "github.com/diasYuri/y/pkg/extensions"
	"github.com/diasYuri/y/pkg/extensions/mcp"
	memoryext "github.com/diasYuri/y/pkg/extensions/memory"
	"github.com/diasYuri/y/pkg/extensions/wasm"
	"github.com/diasYuri/y/pkg/memory/filesystem"
	"github.com/diasYuri/y/pkg/tools"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type SecretLookup func(context.Context, string) (string, error)
type Manager struct {
	safePoint      func(context.Context, string, func() error) error
	Host           *extensionhost.Host
	Registry       *tools.Registry
	WASM           wasm.Manager
	Memory         *memoryext.Extension
	Settings       Settings
	Config         config.Resolved
	Secret         SecretLookup
	DiscoveryError string
	Offline        bool
	mu             sync.Mutex
	grants         map[string]map[wasm.Capability]bool
}

func New(ctx context.Context, cfg config.Resolved, registry *tools.Registry, secret SecretLookup, offline bool) (*Manager, error) {
	return NewWithSafePoint(ctx, cfg, registry, secret, offline, nil)
}

func NewWithSafePoint(ctx context.Context, cfg config.Resolved, registry *tools.Registry, secret SecretLookup, offline bool, safePoint func(context.Context, string, func() error) error) (manager *Manager, err error) {
	settings, err := Load(cfg)
	if err != nil {
		return nil, err
	}
	m := &Manager{safePoint: safePoint, Host: extensionhost.NewHost(registry), Registry: registry, Settings: settings, Config: cfg, Secret: secret, Offline: offline}
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			err = errors.Join(err, m.Close(cleanup))
		}
	}()
	m.grants = make(map[string]map[wasm.Capability]bool)
	for _, module := range settings.WASM {
		m.grants[module.ID] = make(map[wasm.Capability]bool)
		for _, grant := range module.Grants {
			m.grants[module.ID][wasm.Capability(grant)] = true
		}
	}
	wasmCfg := wasm.DefaultConfig()
	wasmCfg.ExtensionDirs = nil
	wasmCfg.HostVersion = "v1.4.0"
	wasmCfg.MaxLoadedModules = 16
	wasmCfg.DefaultTimeoutMS = uint32(m.timeout().Milliseconds())
	wasmCfg.AllowedCapabilities = []wasm.Capability{wasm.CapYTools, wasm.CapLogs}
	wasmCfg.Policy = wasm.PolicyFunc(func(_ context.Context, req wasm.CapabilityRequest) (wasm.Decision, error) {
		return m.allowCapability(req), nil
	})
	wasmCfg.Invoker = wasm.HostInvokerFunc(m.invokeReadTool)
	for _, module := range settings.WASM {
		wasmCfg.ExtensionDirs = append(wasmCfg.ExtensionDirs, filepath.Dir(module.Directory))
	}
	m.WASM = wasm.NewManager(wasmCfg)
	if err = m.WASM.Discover(ctx); err != nil {
		if !offline {
			return nil, err
		}
		m.DiscoveryError = "WASM discovery failed; inspect configured manifests"
		err = nil
	}
	if settings.Memory.Enabled && !offline {
		root := filepath.Join(cfg.Paths.Data, "memory", fmt.Sprintf("%x", sha256.Sum256([]byte(cfg.Paths.Workspace))))
		store, e := filesystem.NewFilesystemStore(root)
		if e != nil {
			return nil, e
		}
		mc := memoryext.DefaultConfig()
		mc.Enabled = true
		mc.Backend = "filesystem"
		mc.Directory = root
		mc.WorkspaceID = cfg.Paths.Workspace
		mc.MaxReadBytes = int64(min(cfg.Int("limits.output_bytes"), 65536))
		mc.MaxContextTokens = int64(min(cfg.Int("limits.context_tokens")/8, 1500))
		mc.AutoExtract = false
		mc.DedicatedTools = true
		mc.FailOpen = settings.Memory.FailOpen
		m.Memory, err = memoryext.NewWithContext(ctx, memoryext.WithConfig(mc), memoryext.WithStore(store))
		if err != nil {
			return nil, err
		}
		if err = m.Host.InstallContext(ctx, m.Memory); err != nil {
			return nil, err
		}
	}
	if !offline {
		servers := []mcp.ServerConfig{}
		for _, server := range settings.MCP {
			if !server.Enabled {
				continue
			}
			if server.Consent != ServerDigest(server) {
				return nil, fmt.Errorf("MCP %s configuration changed; approve enable again", server.Name)
			}
			converted, e := m.serverConfig(ctx, server)
			if e != nil {
				return nil, e
			}
			servers = append(servers, converted)
		}
		if len(servers) > 0 {
			start, cancel := context.WithTimeout(ctx, m.timeout())
			extension, e := mcp.New(start, mcp.Config{Servers: servers})
			cancel()
			if e != nil {
				return nil, e
			}
			if len(extension.Failures()) > 0 {
				_ = extension.Close(context.Background())
				return nil, errors.New("MCP connection failed; use mcp list/doctor and refresh with an available server")
			}
			if err = m.Host.InstallContext(ctx, extension); err != nil {
				_ = extension.Close(context.Background())
				return nil, err
			}
		}
		for _, module := range settings.WASM {
			if !module.Enabled {
				continue
			}
			digest, e := ModuleDigest(ctx, module)
			if e != nil {
				return nil, e
			}
			if module.Consent != digest {
				return nil, fmt.Errorf("WASM %s changed; approve load again", module.ID)
			}
			if !wasm.HostAvailable() {
				return nil, wasm.ErrHostUnavailable
			}
			if e := m.verifyModule(module); e != nil {
				return nil, e
			}
			definitions, e := wasm.ComponentDefinitions(m.WASM, m.Host, module.ID)
			if e != nil {
				return nil, e
			}
			if err = m.Host.AddDefinitions(ctx, definitions); err != nil {
				return nil, err
			}
			after, e := ModuleDigest(ctx, module)
			if e != nil || after != digest {
				return nil, errors.New("WASM changed while loading")
			}
		}
	} else {
		for _, server := range settings.MCP {
			if !server.Enabled || server.ToolCache == "" {
				continue
			}
			var descriptors []tools.ToolDescriptor
			if err = json.Unmarshal([]byte(server.ToolCache), &descriptors); err != nil {
				return nil, errors.New("invalid cached MCP descriptors")
			}
			for _, desc := range descriptors {
				if err = registry.Add(desc, tools.ToolHandlerFunc(func(context.Context, tools.ToolRequest) (tools.ToolResponse, error) {
					return tools.ToolResponse{}, errors.New("offline inspection does not invoke MCP")
				})); err != nil {
					return nil, err
				}
			}
		}
	}
	if err = m.registerManagement(); err != nil {
		return nil, err
	}
	return m, nil
}
func (m *Manager) atSafePoint(ctx context.Context, exceptRun string, apply func() error) error {
	if m.safePoint == nil {
		return apply()
	}
	return m.safePoint(ctx, exceptRun, apply)
}

func (m *Manager) timeout() time.Duration {
	timeout, err := time.ParseDuration(m.Config.String("limits.process_timeout"))
	if err != nil {
		return 2 * time.Minute
	}
	return timeout
}
func (m *Manager) Close(ctx context.Context) error {
	if m == nil {
		return nil
	}
	var err error
	if m.Host != nil {
		err = m.Host.Close(ctx)
	}
	if m.WASM != nil {
		err = errors.Join(err, m.WASM.Close(ctx))
	}
	return err
}
func (m *Manager) serverConfig(ctx context.Context, server Server) (mcp.ServerConfig, error) {
	out := mcp.ServerConfig{Name: server.Name, Transport: mcp.Transport(server.Transport), Command: server.Command, Args: append([]string(nil), server.Args...), Endpoint: server.Endpoint}
	if server.Transport == "stdio" {
		out.Env = map[string]string{}
		for _, key := range []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL"} {
			if value, ok := os.LookupEnv(key); ok {
				out.Env[key] = value
			}
		}
		for key, reference := range server.EnvRefs {
			if m.Secret == nil {
				return out, errors.New("MCP credential resolver unavailable")
			}
			value, err := m.Secret(ctx, reference)
			if err != nil {
				return out, errors.New("MCP credential reference could not be resolved")
			}
			out.Env[key] = value
		}
	}
	if server.CredentialRef != "" {
		if m.Secret == nil {
			return out, errors.New("MCP credential resolver unavailable")
		}
		out.TokenSource = tokenSource{m.Secret, server.CredentialRef}
	}
	return out, nil
}

type tokenSource struct {
	lookup    SecretLookup
	reference string
}

func (t tokenSource) Resolve(ctx context.Context, _ string) (string, error) {
	value, err := t.lookup(ctx, t.reference)
	if err != nil {
		return "", errors.New("MCP credential reference could not be resolved")
	}
	return value, nil
}
func (m *Manager) digestLocked() string {
	active := Settings{Version: 1, Memory: m.Settings.Memory}
	for _, server := range m.Settings.MCP {
		if server.Enabled {
			active.MCP = append(active.MCP, server)
		}
	}
	for _, module := range m.Settings.WASM {
		if module.Enabled {
			active.WASM = append(active.WASM, module)
		}
	}
	raw, _ := json.Marshal(active)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func (m *Manager) Digest() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.digestLocked()
}

func (m *Manager) ToolGeneration() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.toolGenerationLocked(m.Registry.List())
}

func (m *Manager) toolGenerationLocked(descriptors []tools.ToolDescriptor) string {
	raw, _ := json.Marshal(descriptors)
	return fmt.Sprintf("%x", sha256.Sum256(append(raw, []byte(m.digestLocked())...)))
}

// VisualContributions returns the bounded UI declarations attached to loaded
// WASM extensions. They are data only; the TUI owns rendering and sanitization.
func (m *Manager) VisualContributions() []VisualContribution {
	if m == nil || m.Offline {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []VisualContribution
	for _, module := range m.Settings.WASM {
		if !module.Enabled || module.Consent == "" {
			continue
		}
		for _, contribution := range module.UI {
			item := contribution
			item.ID = module.ID + ":" + contribution.ID
			out = append(out, item)
			if len(out) == 32 {
				return out
			}
		}
	}
	return out
}
func (m *Manager) allowCapability(request wasm.CapabilityRequest) wasm.Decision {
	if m.grants[request.ExtensionID][request.Capability] {
		return wasm.DecisionAllow
	}
	return wasm.DecisionDeny
}

// Only classified, non-sensitive reads can be bridged. Effectful guest work must
// use separately visible tools; raw filesystem/network/process/secrets are denied.
func (m *Manager) invokeReadTool(ctx context.Context, extensionID, name string, args json.RawMessage) (wasm.HostToolInvokeResult, error) {
	if execution.IsWorkspaceMutation(name) || strings.HasPrefix(name, "wasm_") || strings.HasPrefix(name, "task_") || strings.HasPrefix(name, "subagent_") {
		return wasm.HostToolInvokeResult{}, errors.New("WASM bridge supports classified read tools only")
	}
	view, ok := agent.RuntimeViewFromContext(ctx)
	if !ok {
		return wasm.HostToolInvokeResult{}, errors.New("WASM host calls require a durable runtime view")
	}
	classified := slices.ContainsFunc(view.Tools().List(), func(tool tools.ToolDescriptor) bool {
		return tool.Name == name && !tool.Sensitive
	})
	if !classified {
		return wasm.HostToolInvokeResult{}, errors.New("WASM read tool is unavailable or sensitive")
	}
	parent, ok := execution.ToolRequestFromContext(ctx)
	if !ok {
		return wasm.HostToolInvokeResult{}, errors.New("WASM host call has no operation scope")
	}
	hash := sha256.Sum256(append([]byte(extensionID+"\x00"+name+"\x00"), args...))
	parent.ID = parent.ID + ":host:" + fmt.Sprintf("%x", hash)
	parent.Name = name
	parent.Arguments = append(json.RawMessage(nil), args...)
	parent.Approval = nil
	if parent.IdempotencyKey == "" {
		return wasm.HostToolInvokeResult{}, errors.New("WASM host call has no durable operation key")
	}
	parent.IdempotencyKey += ":host:" + fmt.Sprintf("%x", hash)
	response, err := view.Tools().Handle(ctx, parent)
	if err != nil {
		return wasm.HostToolInvokeResult{}, err
	}
	out := wasm.HostToolInvokeResult{}
	for _, block := range response.Content {
		out.Content = append(out.Content, wasm.ContentBlock{Type: string(block.Type), Text: block.Text, Data: block.Details})
	}
	return out, nil
}

func (m *Manager) AgentOptions() []agent.Option {
	options := m.Host.AgentOptions()
	options = append(options, agent.WithRuntimeViewSource(m))
	if m.Memory != nil {
		options = append(options, func(a *agent.Agent) {
			agent.WithRuntimeHooks(agent.RuntimeHooks{BeforeTool: func(ctx context.Context, call *ai.ToolCall) error {
				if call.Name != "memory.save" && call.Name != "memory.forget" {
					return nil
				}
				snapshot := a.Snapshot()
				for i := len(snapshot.Transcript) - 1; i >= 0; i-- {
					message := snapshot.Transcript[i]
					if message.Role == ai.RoleUser {
						hook := m.Memory.Hooks().BeforeMessage
						if hook != nil {
							return hook(ctx, &message)
						}
						break
					}
				}
				return nil
			}})(a)
		})
	}
	return options
}

// A stable, content-derived identity lets an explicitly reviewed approval survive
// a CLI restart while the SDK still owns leases on the exact captured handlers.
func (m *Manager) AcquireRuntimeView(ctx context.Context) (*agent.RuntimeView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	view, err := m.Host.AcquireRuntimeView(ctx)
	if err != nil {
		return nil, err
	}
	info := view.Info()
	identity, _ := agent.RuntimeIdentityFromContext(ctx)
	info.ID = "opera:" + identity.RunID + ":" + m.toolGenerationLocked(view.Tools().List())
	wrapped, err := agent.NewRuntimeView(info, view.Tools(), view.ContextResolver(), view.Hooks(), view.Close)
	if err != nil {
		_ = view.Close()
	}
	return wrapped, err
}

func (m *Manager) verifyModule(module Module) error {
	info, err := m.WASM.Get(module.ID)
	if err != nil {
		return err
	}
	actual, err := filepath.EvalSymlinks(info.Dir)
	if err != nil {
		return err
	}
	expected, err := filepath.EvalSymlinks(module.Directory)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.New("ambiguous WASM ID: discovered module is outside its approved directory")
	}
	return nil
}
