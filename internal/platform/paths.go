// Package platform resolves application paths without creating them.
package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Paths struct {
	Workspace     string `json:"workspace"`
	UserConfig    string `json:"user_config"`
	ProjectConfig string `json:"project_config"`
	Data          string `json:"data"`
	Cache         string `json:"cache"`
	Credentials   string `json:"credentials"`
}

// Resolve accepts an environment lookup so tests never depend on user files.
func Resolve(workspace, home, goos string, env func(string) string) (Paths, error) {
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve workspace: %w", err)
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return Paths{}, fmt.Errorf("workspace must be an existing directory")
	}
	var config, data, cache string
	switch goos {
	case "darwin":
		config = filepath.Join(home, "Library", "Application Support")
		data = config
		cache = filepath.Join(home, "Library", "Caches")
	case "windows":
		config = fallback(env("APPDATA"), filepath.Join(home, "AppData", "Roaming"))
		data = fallback(env("LOCALAPPDATA"), filepath.Join(home, "AppData", "Local"))
		cache = filepath.Join(data, "Cache")
	default:
		config = fallback(env("XDG_CONFIG_HOME"), filepath.Join(home, ".config"))
		data = fallback(env("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
		cache = fallback(env("XDG_CACHE_HOME"), filepath.Join(home, ".cache"))
	}
	config = fallback(env("OPERA_CONFIG_DIR"), filepath.Join(config, "opera"))
	data = fallback(env("OPERA_DATA_DIR"), filepath.Join(data, "opera"))
	cache = fallback(env("OPERA_CACHE_DIR"), filepath.Join(cache, "opera"))
	for _, dir := range []string{config, data, cache} {
		if !filepath.IsAbs(dir) {
			return Paths{}, fmt.Errorf("application directories must be absolute")
		}
	}
	return Paths{
		Workspace: workspace, UserConfig: filepath.Join(config, "config.toml"),
		ProjectConfig: filepath.Join(workspace, ".opera", "config.toml"),
		Data:          data, Cache: cache, Credentials: filepath.Join(data, "credentials"),
	}, nil
}

func Discover(workspace string) (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home: %w", err)
	}
	return Resolve(workspace, home, runtime.GOOS, os.Getenv)
}

func fallback(value, other string) string {
	if value != "" {
		return value
	}
	return other
}
