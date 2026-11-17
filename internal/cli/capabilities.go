package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/core"
	operaext "github.com/diasYuri/opera/internal/extensions"
	"path/filepath"
	"strings"
)

func (a App) extensionSecret(cfg config.Resolved) operaext.SecretLookup {
	return func(_ context.Context, reference string) (string, error) {
		store, err := a.OpenStore(cfg.String("credentials.store"), cfg.Paths.Credentials)
		if err != nil {
			return "", err
		}
		value, err := store.Get(reference)
		if err != nil {
			return "", errors.New("extension credential reference is unavailable")
		}
		return value, nil
	}
}
func (a App) capabilities(command string, opts options, cfg config.Resolved, jsonl bool) (code int) {
	args := opts.args[1:]
	if len(args) == 0 {
		args = []string{"list"}
	}
	settings, err := operaext.Load(cfg)
	if err != nil {
		return a.fail(2, err, jsonl)
	}
	ctx := context.Background()
	emit := func(kind string, value any) int {
		if jsonl {
			return a.emit(kind, value)
		}
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return a.fail(1, err, false)
		}
		return a.writeText(string(raw) + "\n")
	}
	action := func(tool string, value any, intent string, offline bool) int {
		raw, err := json.Marshal(value)
		if err != nil {
			return a.fail(2, err, jsonl)
		}
		body, err := json.Marshal(core.Action{Tool: tool, Arguments: raw, Intent: intent})
		if err != nil {
			return a.fail(2, err, jsonl)
		}
		opts.management = true
		opts.offline = offline
		opts.args = []string{"run", string(body)}
		return a.execution("run", opts, cfg, jsonl)
	}
	if command == "mcp" && args[0] == "add" {
		if len(args) < 4 {
			return a.fail(2, errors.New("usage: opera mcp add <name> stdio <command> [args...] | <name> streamable_http <endpoint> [credential-ref]"), jsonl)
		}
		for _, s := range settings.MCP {
			if s.Name == args[1] {
				return a.fail(2, errors.New("MCP name already exists; edit user settings explicitly"), jsonl)
			}
		}
		server := operaext.Server{Name: args[1], Transport: args[2]}
		if args[2] == "stdio" {
			server.Command = args[3]
			server.Args = append([]string(nil), args[4:]...)
		} else {
			server.Endpoint = args[3]
			if len(args) > 4 {
				server.CredentialRef = args[4]
			}
		}
		settings.MCP = append(settings.MCP, server)
		if err := operaext.Save(cfg, settings); err != nil {
			return a.fail(2, err, jsonl)
		}
		return emit("mcp.configured", map[string]string{"name": server.Name, "state": "disabled"})
	}
	if command == "extensions" && args[0] == "add" {
		if len(args) < 3 {
			return a.fail(2, errors.New("usage: opera extensions add <id> <directory> [logs|y_tools ...]"), jsonl)
		}
		dir, err := filepath.Abs(args[2])
		if err != nil {
			return a.fail(2, err, jsonl)
		}
		for _, module := range settings.WASM {
			if module.ID == args[1] {
				return a.fail(2, errors.New("WASM ID already exists"), jsonl)
			}
		}
		module := operaext.Module{ID: args[1], Directory: dir, Grants: append([]string(nil), args[3:]...)}
		if _, err := operaext.ModuleDigest(ctx, module); err != nil {
			return a.fail(2, err, jsonl)
		}
		settings.WASM = append(settings.WASM, module)
		if err := operaext.Save(cfg, settings); err != nil {
			return a.fail(2, err, jsonl)
		}
		return emit("extension.configured", map[string]string{"id": module.ID, "state": "disabled"})
	}
	if command == "mcp" && (args[0] == "enable" || args[0] == "disable" || args[0] == "refresh") {
		if len(args) != 2 {
			return a.fail(2, errors.New("usage: opera mcp enable|disable|refresh <name>"), jsonl)
		}
		for _, server := range settings.MCP {
			if server.Name == args[1] {
				verb := args[0]
				if verb == "refresh" {
					verb = "enable"
				}
				return action("opera_mcp_"+verb, map[string]any{"name": server.Name, "digest": operaext.ServerDigest(server), "configuration": server}, "Activate this exact MCP connection", true)
			}
		}
		return a.fail(2, errors.New("unknown MCP server"), jsonl)
	}
	if command == "extensions" && (args[0] == "load" || args[0] == "unload") {
		if len(args) != 2 {
			return a.fail(2, errors.New("usage: opera extensions load|unload <id>"), jsonl)
		}
		for _, module := range settings.WASM {
			if module.ID == args[1] {
				digest := module.Consent
				var err error
				if args[0] == "load" {
					digest, err = operaext.ModuleDigest(ctx, module)
				}
				if err != nil {
					return a.fail(2, err, jsonl)
				}
				return action("opera_wasm_"+args[0], map[string]any{"name": module.ID, "digest": digest, "configuration": module}, "Activate this exact WASM module and grants", true)
			}
		}
		return a.fail(2, errors.New("unknown WASM module"), jsonl)
	}
	if command == "memory" && (args[0] == "enable" || args[0] == "disable") {
		return action("opera_memory_"+args[0], map[string]any{}, "Configure local persistent memory", true)
	}
	if command == "memory" {
		switch args[0] {
		case "save":
			if len(args) != 2 {
				return a.fail(2, errors.New("usage: opera memory save '<JSON object>'"), jsonl)
			}
			var value map[string]any
			if json.Unmarshal([]byte(args[1]), &value) != nil {
				return a.fail(2, errors.New("memory value must be a JSON object"), jsonl)
			}
			return action("memory.save", value, "Please remember this explicitly requested memory", false)
		case "forget":
			if len(args) != 2 {
				return a.fail(2, errors.New("usage: opera memory forget <id>"), jsonl)
			}
			return action("memory.forget", map[string]string{"id": args[1]}, "Please forget this explicitly requested memory", false)
		case "read":
			if len(args) != 2 {
				return a.fail(2, errors.New("usage: opera memory read <id>"), jsonl)
			}
			return action("memory.read", map[string]string{"id": args[1]}, "Read a memory", false)
		case "search", "recall":
			return action("memory.search", map[string]string{"query": strings.Join(args[1:], " ")}, "Recall relevant historical memory", false)
		case "list":
			return action("memory.list", map[string]string{"scope": "workspace"}, "List memories", false)
		case "status":
		default:
			return a.fail(2, errors.New("usage: opera memory enable|disable|status|save|read|search|recall|list|forget"), jsonl)
		}
	}
	if command == "mcp" && (args[0] == "resources" || args[0] == "prompts" || args[0] == "templates") {
		if len(args) != 2 {
			return a.fail(2, errors.New("usage: opera mcp resources|prompts|templates <server>"), jsonl)
		}
		suffix := args[0] + ".list"
		if args[0] == "templates" {
			suffix = "resources.templates.list"
		}
		return action("mcp."+args[1]+"."+suffix, map[string]any{}, "Inspect MCP capabilities", false)
	}
	if command == "tools" && args[0] == "call" {
		if len(args) != 3 || !json.Valid([]byte(args[2])) {
			return a.fail(2, errors.New("usage: opera tools call <name> '<JSON arguments>'"), jsonl)
		}
		return action(args[1], json.RawMessage(args[2]), "Execute this explicitly selected tool", false)
	}
	if command == "mcp" && args[0] == "call" {
		if len(args) != 3 || !json.Valid([]byte(args[2])) {
			return a.fail(2, errors.New("usage: opera mcp call <namespaced-tool> '<JSON arguments>'"), jsonl)
		}
		return action(args[1], json.RawMessage(args[2]), "Invoke this explicitly selected MCP capability", false)
	}
	if args[0] != "list" && args[0] != "status" && args[0] != "inspect" && args[0] != "schema" && args[0] != "capabilities" && args[0] != "discover" && args[0] != "reload" && args[0] != "tools" && args[0] != "resources" && args[0] != "prompts" {
		return a.fail(2, fmt.Errorf("unknown %s action", command), jsonl)
	}
	if args[0] == "reload" {
		probe, err := core.Open(ctx, cfg, core.Options{Inspection: true})
		if err != nil {
			return a.fail(1, err, jsonl)
		}
		checkErr := probe.Service.CheckReconfiguration(ctx, "")
		closeErr := probe.Close(ctx)
		if err := errors.Join(checkErr, closeErr); err != nil {
			return a.fail(1, err, jsonl)
		}
	}
	runtime, err := core.Open(ctx, cfg, core.Options{Management: true, Offline: args[0] != "reload" && args[0] != "inspect", Secret: a.extensionSecret(cfg)})
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	defer func() {
		if err := runtime.Close(ctx); err != nil {
			code = a.fail(1, err, jsonl)
		}
	}()
	if args[0] == "reload" {
		if _, err := runtime.Command(ctx, "/extensions reload"); err != nil {
			return a.fail(1, err, jsonl)
		}
	}
	report, err := runtime.Extensions.Inspect(ctx)
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	if command == "tools" {
		return emit("tools.schemas", runtime.Extensions.Registry.List())
	}
	if command == "mcp" {
		if args[0] == "tools" {
			var descriptors []json.RawMessage
			for _, server := range report.Servers {
				if len(args) > 1 && server.Name != args[1] {
					continue
				}
				var cached []json.RawMessage
				if server.ToolCache != "" {
					if err := json.Unmarshal([]byte(server.ToolCache), &cached); err != nil {
						return a.fail(1, errors.New("invalid cached MCP descriptors"), jsonl)
					}
				}
				descriptors = append(descriptors, cached...)
			}
			return emit("mcp.tools", descriptors)
		}
		return emit("mcp.configurations", report.Servers)
	}
	if command == "memory" {
		return emit("memory.status", map[string]any{"state": report.MemoryState, "settings": settings.Memory})
	}
	if args[0] == "schema" {
		if len(args) != 2 {
			return a.fail(2, errors.New("usage: opera extensions schema <id>"), jsonl)
		}
		info, err := runtime.Extensions.WASM.Get(args[1])
		if err != nil {
			return a.fail(1, err, jsonl)
		}
		return emit("extension.schema", info.Manifest)
	}
	if args[0] == "capabilities" {
		return emit("extension.capabilities", report.SupportedGrants)
	}
	return emit("extensions.report", map[string]any{"extensions": report, "components": runtime.Harness.Descriptors()})
}
