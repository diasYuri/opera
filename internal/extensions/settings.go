// Package extensions composes explicitly authorized local capabilities.
package extensions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/privatefs"
	"github.com/diasYuri/y/pkg/extensions/wasm"
	"github.com/pelletier/go-toml/v2"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Server struct {
	Name          string            `toml:"name"`
	Transport     string            `toml:"transport"`
	Command       string            `toml:"command,omitempty"`
	Args          []string          `toml:"args,omitempty"`
	Endpoint      string            `toml:"endpoint,omitempty"`
	CredentialRef string            `toml:"credential_ref,omitempty"`
	EnvRefs       map[string]string `toml:"env_refs,omitempty"`
	Enabled       bool              `toml:"enabled"`
	Consent       string            `toml:"consent,omitempty"`
	ToolCache     string            `toml:"tool_cache,omitempty"`
}
type Module struct {
	ID        string               `toml:"id"`
	Directory string               `toml:"directory"`
	Grants    []string             `toml:"grants"`
	Enabled   bool                 `toml:"enabled"`
	Consent   string               `toml:"consent,omitempty"`
	UI        []VisualContribution `toml:"ui,omitempty"`
}

// VisualContribution is a bounded, declarative UI contribution owned by the
// extension settings file. It cannot contain terminal escapes or executable
// rendering code.
type VisualContribution struct {
	ID      string `toml:"id"`
	Kind    string `toml:"kind"`
	Target  string `toml:"target,omitempty"`
	Text    string `toml:"text,omitempty"`
	Detail  string `toml:"detail,omitempty"`
	Trigger string `toml:"trigger,omitempty"`
	Value   string `toml:"value,omitempty"`
}
type Memory struct {
	Enabled  bool `toml:"enabled"`
	FailOpen bool `toml:"fail_open"`
}
type Settings struct {
	Version int      `toml:"version"`
	MCP     []Server `toml:"mcp"`
	WASM    []Module `toml:"wasm"`
	Memory  Memory   `toml:"memory"`
}

func SettingsPath(cfg config.Resolved) string {
	if path := cfg.String("extensions.file"); path != "" {
		return path
	}
	return filepath.Join(cfg.Paths.Data, "extensions", fmt.Sprintf("%x.toml", sha256.Sum256([]byte(cfg.Paths.Workspace))))
}
func Load(cfg config.Resolved) (Settings, error) {
	path := SettingsPath(cfg)
	if !filepath.IsAbs(path) {
		return Settings{}, errors.New("extensions.file must be absolute")
	}
	if cfg.String("extensions.file") == "" {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return Settings{Version: 1}, nil
		}
	}
	if err := outsideWorkspace(cfg, path); err != nil {
		return Settings{}, err
	}
	settings := Settings{Version: 1}
	if err := privatefs.Check(path, false); os.IsNotExist(err) {
		return settings, nil
	} else if err != nil {
		return settings, err
	}
	file, err := os.Open(path)
	if err != nil {
		return settings, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return settings, errors.New("extension settings exceed 1 MiB")
	}
	decoder := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return settings, errors.New("invalid extension TOML; values omitted")
	}
	return settings, settings.Validate()
}
func (s Settings) Validate() error {
	if s.Version != 1 {
		return errors.New("unsupported extension settings version")
	}
	seen := map[string]bool{}
	valid := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]{0,63}$`)
	for _, server := range s.MCP {
		if !valid.MatchString(server.Name) || seen["mcp:"+server.Name] {
			return errors.New("invalid or duplicate MCP name")
		}
		seen["mcp:"+server.Name] = true
		switch server.Transport {
		case "stdio":
			if server.Command == "" || server.Endpoint != "" {
				return errors.New("stdio requires command and no endpoint")
			}
		case "streamable_http":
			u, e := url.Parse(server.Endpoint)
			if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" || u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
				return errors.New("MCP HTTP requires HTTPS or loopback HTTP, without embedded credentials/query")
			}
			if server.Command != "" {
				return errors.New("HTTP transport cannot execute a command")
			}
		default:
			return errors.New("unsupported MCP transport")
		}
	}
	for _, module := range s.WASM {
		if !valid.MatchString(module.ID) || seen["wasm:"+module.ID] || !filepath.IsAbs(module.Directory) {
			return errors.New("invalid WASM identity/directory")
		}
		seen["wasm:"+module.ID] = true
		for _, grant := range module.Grants {
			if grant != "logs" && grant != "y_tools" {
				return errors.New("unsupported host capability; Opera supports logs and classified read tools through y_tools")
			}
		}
		if len(module.UI) > 24 {
			return errors.New("WASM UI contributions exceed 24 entries per module")
		}
		seenUI := map[string]bool{}
		for _, item := range module.UI {
			if !valid.MatchString(item.ID) || seenUI[item.ID] {
				return errors.New("invalid or duplicate WASM UI contribution ID")
			}
			seenUI[item.ID] = true
			if !validContributionText(item.Text) || !validContributionText(item.Detail) || !validContributionText(item.Trigger) || !validContributionText(item.Value) {
				return errors.New("WASM UI contributions must be plain single-line text")
			}
			switch item.Kind {
			case "header", "footer", "widget":
				if item.Text == "" || item.Target != "" || item.Trigger != "" || item.Value != "" {
					return errors.New("header, footer and widget contributions require text only")
				}
			case "renderer":
				if item.Text == "" || item.Trigger != "" || item.Value != "" || item.Target != "assistant" && item.Target != "user" && item.Target != "thinking" && item.Target != "tool" && item.Target != "error" && item.Target != "system" && item.Target != "skill" && item.Target != "agent" && item.Target != "compaction" && item.Target != "branch" && item.Target != "announcement" {
					return errors.New("renderer contribution requires a supported target and label")
				}
			case "autocomplete":
				if item.Target != "" || item.Trigger == "" || item.Value == "" || item.Text == "" {
					return errors.New("autocomplete contribution requires trigger, value and label")
				}
			default:
				return errors.New("unsupported WASM UI contribution kind")
			}
		}
	}
	return nil
}
func Save(cfg config.Resolved, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	path := SettingsPath(cfg)
	if err := outsideWorkspace(cfg, path); err != nil {
		return err
	}
	if _, err := Load(cfg); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := privatefs.Check(dir, true); err != nil {
		return err
	}
	raw, err := toml.Marshal(s)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".extensions-*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if err := privatefs.Protect(name, false); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func ServerDigest(server Server) string {
	server.Enabled = false
	server.Consent = ""
	server.ToolCache = ""
	raw, _ := json.Marshal(server)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func ModuleDigest(ctx context.Context, module Module) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	manifest, err := wasm.ReadManifest(filepath.Join(module.Directory, wasm.ManifestFileName))
	if err != nil {
		return "", err
	}
	if manifest.ID != module.ID {
		return "", errors.New("WASM manifest ID mismatch")
	}
	root, err := os.OpenRoot(module.Directory)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(manifest.Entry)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if n, err := io.Copy(hash, io.LimitReader(file, 64<<20+1)); err != nil {
		return "", err
	} else if n > 64<<20 {
		return "", errors.New("WASM module exceeds 64 MiB")
	}
	module.Consent = ""
	module.Enabled = false
	module.UI = nil
	if len(module.Grants) == 0 {
		module.Grants = nil
	}
	raw, _ := json.Marshal(struct {
		Module   Module
		Manifest wasm.Manifest
	}{module, manifest})
	_, _ = hash.Write(raw)
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func validContributionText(value string) bool {
	if len(value) > 256 || strings.ContainsAny(value, "\x00\r\n\x1b") {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Resolve existing ancestors as well as the final file so a parent symlink cannot
// redirect the user-owned consent store into the repository.
func outsideWorkspace(cfg config.Resolved, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("extension settings path must be absolute")
	}
	workspace, err := canonicalPrefix(cfg.Paths.Workspace)
	if err != nil {
		return err
	}
	actual, err := canonicalPrefix(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(workspace, actual)
	if err != nil || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("extension settings must be outside the workspace")
	}
	return nil
}
func canonicalPrefix(path string) (string, error) {
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = canonicalPrefix(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}
