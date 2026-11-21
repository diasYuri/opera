// Opera development checks use only Go and pinned modules, including on Windows.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const toolchain = "go1.27.4"
const lintVersion = "v2.15.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if runtime.Version() != toolchain {
		return fmt.Errorf("use GOTOOLCHAIN=%s; running %s", toolchain, runtime.Version())
	}
	if len(args) != 1 {
		return errors.New("usage: go run ./scripts <check|lint|matrix|inventory|package|package-smoke>")
	}
	switch args[0] {
	case "check":
		if err := checkSource(); err != nil {
			return err
		}
		if err := checkModules(); err != nil {
			return err
		}
		for _, cmd := range [][]string{{"mod", "verify"}, {"vet", "./..."}, {"test", "./..."}, {"-C", "testdata/sdk-consumer", "test", "-mod=readonly", "./..."}, {"build", "-trimpath", "-o", executable("bin/opera"), "./cmd/opera"}} {
			if err := goCommand(nil, cmd...); err != nil {
				return err
			}
		}
		return nil
	case "lint":
		return goCommand(nil, "run", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@"+lintVersion, "run", "./...")
	case "matrix":
		for _, goos := range []string{"linux", "darwin", "windows"} {
			for _, arch := range []string{"amd64", "arm64"} {
				name := "dist/opera_" + goos + "_" + arch
				if goos == "windows" {
					name += ".exe"
				}
				env := map[string]string{"GOOS": goos, "GOARCH": arch}
				if err := goCommand(env, "build", "-trimpath", "-o", name, "./cmd/opera"); err != nil {
					return err
				}
				// Compile the real dependency consumer and platform-specific tests.
				for _, pkg := range []struct{ name, path string }{{"credentials", "./internal/adapters/credentials"}, {"foundation", "./internal/foundation"}} {
					if err := goCommand(env, "test", "-c", "-o", "dist/"+pkg.name+"_"+goos+"_"+arch+".test", pkg.path); err != nil {
						return err
					}
				}
			}
		}
		return nil
	case "package":
		return packages()
	case "package-smoke":
		return packageSmoke()
	case "inventory":
		return inventory()
	default:
		return errors.New("unknown development check")
	}
}

func goCommand(overrides map[string]string, args ...string) error {
	fmt.Println("go " + strings.Join(args, " "))
	cmd := exec.Command("go", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	env := map[string]string{"GOTOOLCHAIN": toolchain, "GOWORK": "off", "CGO_ENABLED": "0", "GOFLAGS": "-tags=feature_lsp,feature_wasm_ext"}
	for key, value := range overrides {
		env[key] = value
	}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, replaced := env[key]; !replaced {
			cmd.Env = append(cmd.Env, item)
		}
	}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	return cmd.Run()
}

func executable(path string) string {
	if runtime.GOOS == "windows" {
		return path + ".exe"
	}
	return path
}

func checkSource() error {
	return filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != "." && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "bin" || entry.Name() == "dist") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		formatted, err := format.Source(data)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, formatted) {
			return fmt.Errorf("gofmt required: %s", path)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			externalIntegrationTest := strings.HasSuffix(path, "_test.go") && strings.HasSuffix(file.Name.Name, "_test")
			if err := validateImportForFile(path, name, externalIntegrationTest); err != nil {
				return err
			}
		}
		if err := validateCLIProviderAuth(path, string(data)); err != nil {
			return err
		}
		return nil
	})
}

func validateCLIProviderAuth(path, source string) error {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "internal/ui/cli/") && !strings.HasPrefix(path, "internal/core/") {
		return nil
	}
	for _, detail := range []string{"claude.ai/oauth/", "platform.claude.com/", "openrouter.ai/auth", "openrouter.ai/api/v1/auth/keys", "auth.openai.com/", "chatgpt.com/", "grant_type", "code_challenge_method", "code_verifier", "anthropic-oauth", "openrouter-oauth", "chatgpt-oauth", "AnthropicAuthorizationURL", "OpenRouterAuthorizationURL"} {
		if strings.Contains(source, detail) {
			return fmt.Errorf("provider-specific OAuth protocol belongs in its adapter (%s contains %q)", path, detail)
		}
	}
	return nil
}

func validateImport(path, name string) error {
	return validateImportForFile(path, name, false)
}

func validateImportForFile(path, name string, externalIntegrationTest bool) error {
	path = filepath.ToSlash(path)
	if strings.HasPrefix(name, "github.com/diasYuri/y/") && strings.Contains(name, "/internal/") {
		return fmt.Errorf("private Y import in %s", path)
	}
	if !externalIntegrationTest && strings.HasPrefix(path, "internal/modules/accounting/") && strings.HasPrefix(name, "github.com/diasYuri/opera/internal/adapters/persistence/sqlite") {
		return fmt.Errorf("accounting must write usage through its module contract (%s)", path)
	}
	if strings.HasPrefix(path, "internal/modules/") {
		for _, blocked := range []string{"github.com/diasYuri/opera/internal/core", "github.com/diasYuri/opera/internal/ui", "github.com/diasYuri/opera/internal/ui/cli", "github.com/diasYuri/opera/internal/adapters/providers/"} {
			if externalIntegrationTest && strings.HasPrefix(blocked, "github.com/diasYuri/opera/internal/adapters/providers/") {
				continue
			}
			if strings.HasPrefix(name, blocked) {
				return fmt.Errorf("modules must not depend on composition, UI, CLI, or concrete providers: %s (%s)", name, path)
			}
		}
	}
	if strings.HasPrefix(path, "internal/adapters/") && (strings.HasPrefix(name, "github.com/diasYuri/opera/internal/core") || strings.HasPrefix(name, "github.com/diasYuri/opera/internal/ui")) {
		return fmt.Errorf("adapters must not depend on core or UI (%s)", path)
	}
	if !externalIntegrationTest && strings.HasPrefix(name, "github.com/diasYuri/opera/internal/adapters/providers/") && !strings.HasPrefix(path, "internal/core/") && !strings.HasPrefix(path, "internal/adapters/providers/") {
		return fmt.Errorf("providers may be imported only by core composition and their own modules, not %s (%s)", name, path)
	}
	if strings.HasPrefix(path, "internal/ui/tui/") {
		for _, blocked := range []string{"github.com/diasYuri/opera/internal/adapters/", "github.com/diasYuri/opera/internal/core", "github.com/diasYuri/opera/internal/ui/cli", "github.com/diasYuri/y/pkg/providers", "github.com/diasYuri/y/pkg/agent", "github.com/diasYuri/y/pkg/tools", "os/exec"} {
			if externalIntegrationTest && blocked == "github.com/diasYuri/opera/internal/adapters/" && strings.HasPrefix(name, "github.com/diasYuri/opera/internal/adapters/providers/") {
				continue
			}
			if strings.HasPrefix(name, blocked) {
				return fmt.Errorf("TUI must depend on product actions/events, not %s (%s)", name, path)
			}
		}
	}
	if strings.HasPrefix(path, "internal/ui/cli/") {
		for _, blocked := range []string{"github.com/diasYuri/opera/internal/adapters/providers/", "github.com/diasYuri/y/pkg/agent", "github.com/diasYuri/y/pkg/tools"} {
			if externalIntegrationTest && strings.HasPrefix(blocked, "github.com/diasYuri/opera/internal/adapters/providers/") {
				continue
			}
			if externalIntegrationTest && blocked == "github.com/diasYuri/y/pkg/agent" && strings.HasPrefix(name, "github.com/diasYuri/y/pkg/agent/agenttest") {
				continue
			}
			if strings.HasPrefix(name, blocked) {
				return fmt.Errorf("CLI must use core composition and product modules, not %s (%s)", name, path)
			}
		}
	}
	return nil
}

func checkModules() error {
	for _, name := range []string{"go.work", "go.work.sum"} {
		if _, err := os.Stat(name); err == nil {
			return fmt.Errorf("remove local integration file before acceptance: %s", name)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	data, err := exec.Command("go", "mod", "edit", "-json").Output()
	if err != nil {
		return err
	}
	var mod struct {
		Go      string
		Replace []json.RawMessage
		Require []struct{ Path, Version string }
	}
	if err := json.Unmarshal(data, &mod); err != nil {
		return err
	}
	if len(mod.Replace) != 0 || "go"+mod.Go != toolchain {
		return errors.New("module must pin the required Go patch and contain no replacements")
	}
	expected := map[string]string{
		"charm.land/bubbletea/v2": "v2.1.2", "charm.land/bubbles/v2": "v2.2.2",
		"charm.land/huh/v2": "v2.0.3", "charm.land/lipgloss/v2": "v2.1.1",
		"github.com/charmbracelet/harmonica": "v0.2.0", "github.com/diasYuri/y": "v1.4.0",
	}
	for _, dep := range mod.Require {
		if want, ok := expected[dep.Path]; ok {
			if dep.Version != want {
				return fmt.Errorf("unexpected version for %s", dep.Path)
			}
			delete(expected, dep.Path)
		}
	}
	if len(expected) != 0 {
		return errors.New("missing pinned foundation dependency")
	}
	return nil
}
