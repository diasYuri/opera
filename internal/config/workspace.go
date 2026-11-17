package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diasYuri/opera/internal/platform"
	"github.com/diasYuri/opera/internal/privatefs"
	"github.com/pelletier/go-toml/v2"
)

func WorkspaceSettingsPath(paths platform.Paths) string {
	if !filepath.IsAbs(paths.Data) || paths.Workspace == "" {
		return ""
	}
	return filepath.Join(paths.Data, "workspaces", fmt.Sprintf("%x.toml", sha256.Sum256([]byte(paths.Workspace))))
}

// SaveWorkspaceSelection persists user decisions outside version-controlled
// project configuration. It is the workspace-specific part of the user layer.
func SaveWorkspaceSelection(paths platform.Paths, provider, model, policy string) error {
	if err := ValidateWorkspaceSelection(provider, model, policy); err != nil {
		return err
	}
	return saveWorkspaceSettings(paths, map[string]any{"provider": provider, "model": model, "policy": policy})
}

func SaveWorkspaceCredentialStore(paths platform.Paths, kind string) error {
	if kind != "keyring" && kind != "file" {
		return fmt.Errorf("unknown credential store")
	}
	return saveWorkspaceSettings(paths, map[string]any{"credentials": map[string]any{"store": kind}})
}

// SaveWorkspacePreference persists a validated user choice outside the
// version-controlled project settings. Only interactive preferences are
// writable here; credential and infrastructure settings stay on their own
// configuration paths.
func SaveWorkspacePreference(paths platform.Paths, key, raw string) error {
	if key != "thinking" && key != "policy" && key != "ui.theme" {
		return fmt.Errorf("workspace preference is not editable here")
	}
	var selected *Field
	for _, field := range Schema() {
		if field.Key == key {
			selected = &field
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("unknown workspace preference")
	}
	value, err := selected.parse(raw)
	if err != nil {
		return err
	}
	updates := map[string]any{}
	parts := strings.Split(key, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		if i == len(parts)-1 {
			updates[parts[i]] = value
		} else {
			updates = map[string]any{parts[i]: updates}
		}
	}
	return saveWorkspaceSettings(paths, updates)
}

func saveWorkspaceSettings(paths platform.Paths, updates map[string]any) error {
	path := WorkspaceSettingsPath(paths)
	if path == "" {
		return fmt.Errorf("workspace settings path is unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := privatefs.Protect(filepath.Dir(path), true); err != nil {
		return err
	}
	if err := privatefs.Check(path, false); err != nil && !os.IsNotExist(err) {
		return err
	}
	values := map[string]any{}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(existing) > 0 {
		if toml.Unmarshal(existing, &values) != nil {
			return fmt.Errorf("invalid workspace settings; existing file was preserved")
		}
	}
	mergeSettings(values, updates)
	raw, err := toml.Marshal(values)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	if err := privatefs.Protect(file.Name(), false); err != nil {
		return err
	}
	if _, err := file.Write(raw); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// mergeSettings preserves sibling values in nested TOML sections such as ui.
func mergeSettings(target, updates map[string]any) {
	for key, value := range updates {
		incoming, isMap := value.(map[string]any)
		if !isMap {
			target[key] = value
			continue
		}
		current, ok := target[key].(map[string]any)
		if !ok {
			current = map[string]any{}
			target[key] = current
		}
		mergeSettings(current, incoming)
	}
}

// ValidateWorkspaceSelection checks an explicit selection without writing it.
func ValidateWorkspaceSelection(provider, model, policy string) error {
	for _, field := range Schema() {
		if value, ok := map[string]string{"provider": provider, "model": model, "policy": policy}[field.Key]; ok {
			if _, err := field.parse(value); err != nil {
				return err
			}
		}
	}
	return nil
}
