package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/extensions/wasm"
)

type Report struct {
	DiscoveryError  string                      `json:"discovery_error,omitempty"`
	SettingsPath    string                      `json:"settings_path"`
	Offline         bool                        `json:"offline"`
	WASMAvailable   bool                        `json:"wasm_available"`
	WASM            []wasm.ExtensionInfo        `json:"wasm"`
	MemoryState     string                      `json:"memory_state"`
	Servers         []Server                    `json:"servers"`
	View            agent.RuntimeViewInfo       `json:"view"`
	States          []component.ComponentStatus `json:"states"`
	SupportedGrants []string                    `json:"supported_grants"`
}

func (m *Manager) Inspect(ctx context.Context) (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := Report{DiscoveryError: m.DiscoveryError, SettingsPath: SettingsPath(m.Config), Offline: m.Offline, WASMAvailable: wasm.HostAvailable(), WASM: m.WASM.List(), Servers: append([]Server(nil), m.Settings.MCP...), MemoryState: "disabled", SupportedGrants: []string{"logs", "y_tools (classified reads only)"}}
	if m.Memory != nil {
		out.MemoryState = string(m.Memory.PersistenceStatus())
	} else if m.Settings.Memory.Enabled {
		out.MemoryState = "enabled-offline"
	}
	view, err := m.Host.AcquireRuntimeView(ctx)
	if err != nil {
		return out, err
	}
	out.View = view.Info()
	if err := view.Close(); err != nil {
		return out, err
	}
	for _, id := range []string{"memory", "mcp"} {
		if status, ok := m.Host.Status(id); ok {
			out.States = append(out.States, status)
		}
	}
	for _, module := range m.Settings.WASM {
		if status, ok := m.Host.Status("wasm." + module.ID); ok {
			out.States = append(out.States, status)
		}
	}
	return out, nil
}

// The SDK supports transactional generation changes for scoped components.
// Replacing aggregate MCP/memory instances or changing a warm WASM module's
// bytes/grants requires a new CLI scope; rejected transitions keep the old host.
func (m *Manager) Reload(ctx context.Context) error {
	return m.atSafePoint(ctx, "", func() error { return m.reload(ctx) })
}

func (m *Manager) reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next, err := Load(m.Config)
	if err != nil {
		return err
	}
	oldStable, _ := json.Marshal(struct {
		MCP    []Server
		Memory Memory
	}{m.Settings.MCP, m.Settings.Memory})
	nextStable, _ := json.Marshal(struct {
		MCP    []Server
		Memory Memory
	}{next.MCP, next.Memory})
	if string(oldStable) != string(nextStable) {
		return errors.New("MCP/memory replacement requires a new CLI scope; current generation is preserved")
	}
	if len(next.WASM) != len(m.Settings.WASM) {
		return errors.New("WASM directory discovery changes require a new CLI scope")
	}
	changes := []Module{}
	for i, module := range next.WASM {
		old := m.Settings.WASM[i]
		if module.ID != old.ID || module.Directory != old.Directory {
			return errors.New("WASM identity/directory replacement requires restart")
		}
		old.Enabled = module.Enabled
		old.Consent = module.Consent
		rawA, _ := json.Marshal(old)
		rawB, _ := json.Marshal(module)
		if string(rawA) != string(rawB) {
			return errors.New("WASM grant changes require unload and a new CLI scope")
		}
		if module.Enabled {
			digest, err := ModuleDigest(ctx, module)
			if err != nil {
				return err
			}
			if digest != module.Consent {
				return fmt.Errorf("WASM %s changed; approve load again", module.ID)
			}
		}
		if module.Enabled != m.Settings.WASM[i].Enabled {
			changes = append(changes, module)
		}
	}
	if len(changes) > 1 {
		return errors.New("reload supports one WASM activation transition at a time")
	}
	if len(changes) == 0 {
		return nil
	}
	module := changes[0]
	if module.Enabled {
		if err := m.verifyModule(module); err != nil {
			return err
		}
		definitions, err := wasm.ComponentDefinitions(m.WASM, m.Host, module.ID)
		if err != nil {
			return err
		}
		return m.commitSettings(next, m.Host.AddDefinitions(ctx, definitions))
	}
	return m.commitSettings(next, m.Host.Uninstall(ctx, "wasm."+module.ID))
}

func (m *Manager) commitSettings(next Settings, err error) error {
	var reconcile *component.ReconcileError
	if err == nil || (errors.As(err, &reconcile) && reconcile.Committed) {
		m.Settings = next
	}
	return err
}
