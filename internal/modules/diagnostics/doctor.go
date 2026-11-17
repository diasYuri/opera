// Package diagnostics performs local checks without starting providers or tools.
package diagnostics

import (
	"runtime"

	"github.com/diasYuri/opera/internal/modules/configuration"
	operaext "github.com/diasYuri/opera/internal/modules/extensions"
	"github.com/diasYuri/y/pkg/extensions/wasm"
)

const GoVersion = "go1.27.4"
const YVersion = "v1.3.4"

type Check struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Fallback string `json:"fallback,omitempty"`
}

type Report struct {
	Go              string  `json:"go"`
	SDK             string  `json:"y_sdk"`
	CredentialStore string  `json:"credential_store"`
	Checks          []Check `json:"checks"`
}

func Collect(cfg config.Resolved, lookup func(string) (string, error)) Report {
	r := Report{Go: runtime.Version(), SDK: YVersion, CredentialStore: cfg.String("credentials.store")}
	status := "ok"
	if runtime.Version() != GoVersion {
		status = "error"
	}
	r.Checks = append(r.Checks, Check{"toolchain", status, "required: " + GoVersion, ""})
	for _, tool := range []struct{ name, fallback string }{
		{"git", "Git tools will be unavailable; filesystem inspection remains available ."},
		{"rg", "Use the filesystem search tool as a text-search fallback."},
		{"ast-grep", "AST search will be unavailable; use text search ."},
		{"rust-analyzer", "Use file/text search when Rust navigation is unavailable."},
		{"typescript-language-server", "Use file/text search when JS/TS navigation is unavailable."},
		{"basedpyright-langserver", "Use file/text search when Python navigation is unavailable."},
		{"clangd", "Use file/text search when C/C++ navigation is unavailable."},
		{"gopls", "Go semantic navigation will be unavailable; use file/text search ."},
	} {
		name := tool.name
		path, err := lookup(name)
		if err != nil {
			r.Checks = append(r.Checks, Check{name, "missing", "optional external dependency", tool.fallback})
		} else {
			r.Checks = append(r.Checks, Check{name, "ok", path, ""})
		}
	}
	if cfg.String("provider") == "" || cfg.String("model") == "" {
		r.Checks = append(r.Checks, Check{"model", "unconfigured", "Choose provider and model using onboarding, environment or flags.", "Local help, config and doctor do not require credentials."})
	} else {
		r.Checks = append(r.Checks, Check{"model", "configured", "Provider/model selection is configured; remote access has not been tested.", ""})
	}
	r.Checks = append(r.Checks,
		Check{"execution", "ready", "Chat/run/resume use the public durable harness. Doctor started no provider or tool.", ""},
		Check{"lsp", "configured", "Scoped, lazy-started language servers require a build with feature_lsp; make build enables it.", "Use filesystem search/read if no server is installed."},
		Check{"wasm", wasmStatus(), "Local host requires feature_wasm_ext; make build enables it. No module was loaded.", "Use a build with feature_wasm_ext."},
		Check{"credentials", "configured", "Selected store only; no secret or native keyring was read. Use auth status to check a reference.", ""},
	)
	settings, err := operaext.Load(cfg)
	if err != nil {
		r.Checks = append(r.Checks, Check{"extensions", "error", err.Error(), "Inspect user-owned extension settings; no external code was started."})
	} else {
		r.Checks = append(r.Checks, Check{"extensions", "configured", operaext.SettingsPath(cfg), "Activation consent is bound to configuration and module bytes; use extensions inspect for generations."})
		for _, server := range settings.MCP {
			state := "disabled"
			if server.Enabled {
				state = "configured-unprobed"
				if server.Consent != operaext.ServerDigest(server) {
					state = "consent-required"
				}
			}
			r.Checks = append(r.Checks, Check{"mcp." + server.Name, state, server.Transport, "Use mcp refresh to explicitly probe and approve the connection."})
		}
		state := "disabled"
		if settings.Memory.Enabled {
			state = "configured-unprobed"
		}
		r.Checks = append(r.Checks, Check{"memory", state, "Filesystem persistence; readiness is checked on activation, writes remain required.", ""})
	}
	return r
}
func wasmStatus() string {
	if wasm.HostAvailable() {
		return "compiled"
	}
	return "disabled-by-build"
}
