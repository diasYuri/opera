package extensions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/y/pkg/extensions/mcp"
	"github.com/diasYuri/y/pkg/extensions/wasm"
	"github.com/diasYuri/y/pkg/memory/filesystem"
	"github.com/diasYuri/y/pkg/tools"
	"path/filepath"
)

func (m *Manager) registerManagement() error {
	for _, name := range []string{"opera_mcp_enable", "opera_mcp_disable", "opera_wasm_load", "opera_wasm_unload", "opera_memory_enable", "opera_memory_disable"} {
		name := name
		descriptor := tools.ToolDescriptor{Name: name, Description: "Apply an explicit user-owned capability configuration change. Consent binds its exact configuration digest; activation may start external code.", Sensitive: true, InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"digest":{"type":"string"},"configuration":{"type":"object"}},"additionalProperties":false}`)}
		if err := m.Registry.Add(descriptor, tools.ToolHandlerFunc(func(ctx context.Context, req tools.ToolRequest) (tools.ToolResponse, error) {
			var response tools.ToolResponse
			err := m.atSafePoint(ctx, req.RunID, func() error {
				var err error
				response, err = m.applyManagement(ctx, name, req)
				return err
			})
			return response, err
		})); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) applyManagement(ctx context.Context, name string, req tools.ToolRequest) (tools.ToolResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var args struct{ Name, Digest string }
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		return tools.ToolResponse{}, err
	}
	next, err := Load(m.Config)
	if err != nil {
		return tools.ToolResponse{}, err
	}
	switch name {
	case "opera_mcp_enable", "opera_mcp_disable":
		found := false
		for i, server := range next.MCP {
			if server.Name != args.Name {
				continue
			}
			found = true
			if args.Digest != ServerDigest(server) {
				return tools.ToolResponse{}, errors.New("MCP configuration changed; submit a new activation")
			}
			if name == "opera_mcp_enable" {
				converted, err := m.serverConfig(ctx, server)
				if err != nil {
					return tools.ToolResponse{}, err
				}
				start, cancel := context.WithTimeout(ctx, m.timeout())
				extension, err := mcp.New(start, mcp.Config{Servers: []mcp.ServerConfig{converted}})
				cancel()
				if err != nil {
					return tools.ToolResponse{}, errors.New("MCP could not connect")
				}
				defer func() { _ = extension.Close(context.Background()) }()
				if len(extension.Failures()) > 0 {
					return tools.ToolResponse{}, errors.New("MCP connection/discovery failed")
				}
				registry := tools.NewRegistry()
				defer func() { _ = registry.Close(context.Background()) }()
				if err := extension.RegisterTools(registry); err != nil {
					return tools.ToolResponse{}, err
				}
				raw, err := json.Marshal(registry.List())
				if err != nil {
					return tools.ToolResponse{}, err
				}
				next.MCP[i].ToolCache = string(raw)
				next.MCP[i].Enabled = true
				next.MCP[i].Consent = args.Digest
			} else {
				next.MCP[i].Enabled = false
				next.MCP[i].Consent = ""
			}
		}
		if !found {
			return tools.ToolResponse{}, errors.New("unknown MCP server")
		}
	case "opera_wasm_load", "opera_wasm_unload":
		found := false
		for i, module := range next.WASM {
			if module.ID != args.Name {
				continue
			}
			found = true
			digest := module.Consent
			var err error
			if name == "opera_wasm_load" {
				digest, err = ModuleDigest(ctx, module)
			}
			if err != nil {
				return tools.ToolResponse{}, err
			}
			if digest != args.Digest {
				return tools.ToolResponse{}, errors.New("WASM changed; submit a new activation")
			}
			if name == "opera_wasm_load" {
				if !wasm.HostAvailable() {
					return tools.ToolResponse{}, wasm.ErrHostUnavailable
				}
				if err := m.verifyModule(module); err != nil {
					return tools.ToolResponse{}, err
				}
				if err := m.probeModule(ctx, module); err != nil {
					return tools.ToolResponse{}, err
				}
				after, err := ModuleDigest(ctx, module)
				if err != nil || after != digest {
					return tools.ToolResponse{}, errors.New("WASM changed while loading")
				}
				next.WASM[i].Enabled = true
				next.WASM[i].Consent = digest
			} else {
				next.WASM[i].Enabled = false
				next.WASM[i].Consent = ""
			}
		}
		if !found {
			return tools.ToolResponse{}, errors.New("unknown WASM extension")
		}
	case "opera_memory_enable":
		root := filepath.Join(m.Config.Paths.Data, "memory", fmt.Sprintf("%x", sha256.Sum256([]byte(m.Config.Paths.Workspace))))
		store, err := filesystem.NewFilesystemStore(root)
		if err != nil {
			return tools.ToolResponse{}, err
		}
		if err := store.CheckReadiness(ctx); err != nil {
			return tools.ToolResponse{}, err
		}
		next.Memory.Enabled = true
	case "opera_memory_disable":
		next.Memory.Enabled = false
	}
	if err := Save(m.Config, next); err != nil {
		return tools.ToolResponse{}, err
	}
	return tools.ToolResponse{Content: []tools.ContentBlock{{Type: tools.ContentText, Text: "Configuration committed. New activations apply on the next idle reload or CLI invocation."}}}, nil
}

func (m *Manager) probeModule(ctx context.Context, module Module) error {
	if err := m.WASM.Load(ctx, module.ID); err != nil {
		return err
	}
	return m.WASM.Unload(ctx, module.ID)
}
