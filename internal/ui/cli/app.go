// Package cli contains the local CLI adapter. Agent execution belongs in the
// application service introduced in F1, not in command handlers.
package cli

import (
	"errors"
	"fmt"
	"github.com/diasYuri/opera/internal/core"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"golang.org/x/term"

	"context"
	"github.com/diasYuri/opera/internal/adapters/context/filesystem"
	"github.com/diasYuri/opera/internal/adapters/credentials"
	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	"github.com/diasYuri/opera/internal/ui/cli/output"
)

const (
	ExitOK              = 0
	ExitFailure         = 1
	ExitUsage           = 2
	ExitApprovalPending = 3
	ExitInDoubt         = 4
	ExitCancelled       = 130
)

type App struct {
	In              io.Reader
	Out, Err        io.Writer
	Version         string
	IsTTY           bool
	Getwd           func() (string, error)
	Env             func(string) (string, bool)
	Paths           func(string) (platform.Paths, error)
	LookPath        func(string) (string, error)
	OpenStore       func(string, string) (credentials.Store, error)
	ProviderCatalog *providercatalog.Catalog
}

func New(in *os.File, out, stderr io.Writer, version string) App {
	return App{In: in, Out: out, Err: stderr, Version: version,
		IsTTY: terminalPair(in, out), Getwd: os.Getwd, Env: os.LookupEnv,
		Paths: platform.Discover, LookPath: exec.LookPath, OpenStore: credentials.Open}
}

func (a App) Run(args []string) int {
	opts, err := parse(args)
	if err != nil {
		return a.fail(ExitUsage, err, a.jsonRequested(opts))
	}
	if opts.help {
		return a.writeText(help + "\n" + execution.CommandHelp())
	}
	if opts.version {
		return a.writeText("opera " + a.Version + "\n")
	}
	if len(opts.args) == 0 {
		opts.args = []string{"chat"}
	}
	command := opts.args[0]
	if command == "help" {
		return a.writeText(help + "\n" + execution.CommandHelp())
	}
	if command == "config" && len(opts.args) == 2 && opts.args[1] == "schema" {
		if a.jsonRequested(opts) {
			return a.emit("config.schema", config.Schema())
		}
		for _, field := range config.Schema() {
			if _, err := fmt.Fprintf(a.Out, "%s (%s) default=%v env=%s\n", field.Key, field.Kind, field.Default, config.EnvironmentKey(field.Key)); err != nil {
				return ExitFailure
			}
		}
		return ExitOK
	}
	if command != "usage" && command != "models" && command != "demo" && command != "tools" && command != "mcp" && command != "extensions" && command != "memory" && command != "skills" && command != "sessions" && command != "worker" && command != "retry" && command != "compact" && command != "context" && command != "settings" && command != "config" && command != "doctor" && command != "auth" && command != "chat" && command != "run" && command != "resume" && command != "onboarding" && command != "reconcile" {
		return a.fail(ExitUsage, errors.New("unknown command; see --help"), a.jsonRequested(opts))
	}
	paths, err := a.resolvePaths(opts)
	if err != nil {
		return a.fail(ExitUsage, err, a.jsonRequested(opts))
	}
	cfg, err := config.Load(config.Options{Paths: paths, Env: a.Env, Flags: opts.values})
	if err != nil {
		return a.fail(ExitUsage, err, a.jsonRequested(opts))
	}
	jsonl := cfg.String("output.format") == "jsonl"
	if a.ProviderCatalog == nil {
		a.ProviderCatalog, err = core.DefaultCatalog(cfg.CustomProviders...)
		if err != nil {
			return a.fail(ExitUsage, err, jsonl)
		}
	}
	switch command {
	case "tools", "mcp", "extensions", "memory":
		return a.capabilities(command, opts, cfg, jsonl)
	case "skills":
		return a.skills(opts.args[1:], cfg, jsonl)
	case "usage":
		return a.usage(opts, cfg, jsonl)
	case "models":
		return a.models(opts, cfg, jsonl)
	case "demo":
		return a.demo(opts, jsonl)
	case "sessions":
		return a.sessions(opts, cfg, jsonl)
	case "compact":
		return a.compact(opts, cfg, jsonl)
	case "settings":
		return a.config([]string{"show"}, cfg, jsonl)
	case "context":
		bundle, err := contextfiles.Load(context.Background(), cfg.Paths.Workspace, append(opts.attachments, opts.args[1:]...), min(cfg.Int("limits.context_tokens")*2, cfg.Int("limits.attachment_bytes"), 256*1024))
		if err != nil {
			return a.fail(2, err, jsonl)
		}
		if jsonl {
			return a.emit("context.sources", bundle.Sources)
		}
		for _, source := range bundle.Sources {
			if _, err := fmt.Fprintf(a.Out, "%s (%s, %d bytes)\n", source.Path, source.Kind, source.Bytes); err != nil {
				return 1
			}
		}
		return 0
	case "config":
		return a.config(opts.args[1:], cfg, jsonl)
	case "doctor":
		if len(opts.args) != 1 {
			return a.fail(ExitUsage, errors.New("usage: opera doctor"), jsonl)
		}
		return a.doctor(cfg, jsonl)
	case "chat", "run", "worker", "retry", "resume", "onboarding", "reconcile":
		return a.execution(command, opts, cfg, jsonl)
	case "auth":
		if len(opts.args) == 3 {
			if handler := a.authenticator(cfg, opts.args[2]); handler != nil && handler.Supports(opts.args[1]) {
				return a.providerAuth(opts.args[1], opts.args[2], cfg, jsonl, handler)
			}
		}
		if len(opts.args) > 1 && opts.args[1] == "login" {
			return a.oauthLogin(opts.args[2:], cfg, jsonl)
		}
		return a.auth(opts.args[1:], cfg, jsonl)
	}
	return ExitUsage
}

func (a App) resolvePaths(opts options) (platform.Paths, error) {
	workspace := opts.workspace
	if workspace == "" {
		workspace, _ = a.Env("OPERA_WORKSPACE")
	}
	if workspace == "" {
		var err error
		workspace, err = a.Getwd()
		if err != nil {
			return platform.Paths{}, fmt.Errorf("resolve working directory: %w", err)
		}
	}
	paths, err := a.Paths(workspace)
	if err != nil {
		return platform.Paths{}, err
	}
	for _, override := range []struct {
		value  string
		target *string
	}{{opts.userConfig, &paths.UserConfig}, {opts.projectConfig, &paths.ProjectConfig}} {
		if override.value != "" {
			*override.target, err = filepath.Abs(override.value)
			if err != nil {
				return platform.Paths{}, fmt.Errorf("resolve configuration path: %w", err)
			}
			if _, err := os.Stat(*override.target); err != nil {
				return platform.Paths{}, fmt.Errorf("explicit configuration path: %w", err)
			}
		}
	}
	return paths, nil
}

func (a App) config(args []string, cfg config.Resolved, jsonl bool) int {
	if len(args) == 0 || len(args) == 1 && args[0] == "show" {
		if jsonl {
			return a.emit("config.resolved", cfg)
		}
		keys := make([]string, 0, len(cfg.Values))
		for key := range cfg.Values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := cfg.Values[key]
			if _, err := fmt.Fprintf(a.Out, "%s = %v (%s)\n", key, value.Value, value.Source); err != nil {
				return ExitFailure
			}
		}
		return ExitOK
	}
	if len(args) == 1 && args[0] == "paths" {
		if jsonl {
			return a.emit("config.paths", cfg.Paths)
		}
		paths := cfg.Paths
		return a.writeText(fmt.Sprintf("workspace: %s\nconfig directory: %s\nsettings: %s\ncustom providers: %s\nproject config: %s\ndata: %s\ncache: %s\ncredentials: %s\n",
			paths.Workspace, paths.ConfigDir, paths.UserConfig, paths.ProvidersConfig, paths.ProjectConfig, paths.Data, paths.Cache, paths.Credentials))
	}
	if len(args) == 2 && args[0] == "get" {
		value, ok := cfg.Values[args[1]]
		if !ok {
			return a.fail(ExitUsage, errors.New("unknown configuration key; see config schema"), jsonl)
		}
		if jsonl {
			return a.emit("config.value", value)
		}
		return a.writeText(fmt.Sprintf("%v (%s)\n", value.Value, value.Source))
	}
	return a.fail(ExitUsage, errors.New("usage: opera config [show|paths|schema|get <key>]"), jsonl)
}

func (a App) jsonRequested(opts options) bool {
	if value, ok := opts.values["output.format"]; ok {
		return value == "jsonl"
	}
	value, _ := a.Env("OPERA_OUTPUT_FORMAT")
	return value == "jsonl"
}

func (a App) emit(kind string, data any) int {
	if err := output.WriteResult(a.Out, kind, data); err != nil {
		return ExitFailure
	}
	return ExitOK
}

func (a App) writeText(text string) int {
	if _, err := io.WriteString(a.Out, text); err != nil {
		return ExitFailure
	}
	return ExitOK
}

func (a App) fail(code int, err error, jsonl bool) int {
	if jsonl {
		_ = output.WriteResult(a.Err, "error", map[string]any{"message": err.Error(), "exit_code": code})
	} else {
		_, _ = fmt.Fprintln(a.Err, "opera: "+err.Error())
	}
	return code
}

func terminalPair(in *os.File, out io.Writer) bool {
	file, ok := out.(*os.File)
	return ok && term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(file.Fd()))
}
