package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/pelletier/go-toml/v2"

	"github.com/diasYuri/opera/internal/adapters/platform"
)

type Value struct {
	Value  any    `json:"value"`
	Source string `json:"source"`
}

type Resolved struct {
	Values          map[string]Value `json:"values"`
	Paths           platform.Paths   `json:"paths"`
	CustomProviders []CustomProvider `json:"-"`
}

type Options struct {
	Paths platform.Paths
	Env   func(string) (string, bool)
	Flags map[string]string
}

func Load(opts Options) (Resolved, error) {
	result := Resolved{Values: make(map[string]Value), Paths: opts.Paths}
	fields := make(map[string]Field)
	for _, field := range Schema() {
		fields[field.Key] = field
		result.Values[field.Key] = Value{field.Default, "default"}
	}
	for _, layer := range []struct{ path, source string }{{opts.Paths.UserConfig, "user"}, {WorkspaceSettingsPath(opts.Paths), "user-workspace"}, {opts.Paths.ProjectConfig, "project"}} {
		if layer.path == "" {
			continue
		}
		if layer.source == "user-workspace" {
			if err := platform.Check(layer.path, false); err != nil && !os.IsNotExist(err) {
				return Resolved{}, err
			}
		}
		values, err := readConfigFile(layer.path, fields)
		if err != nil {
			return Resolved{}, err
		}
		if layer.source == "project" {
			if policy, ok := values["policy"]; ok && policyRank(policy) > policyRank(result.String("policy")) {
				return Resolved{}, fmt.Errorf("project policy cannot elevate user permissions; choose policy in user configuration, environment or flags")
			}
			if _, ok := values["accounting.prices_file"]; ok {
				return Resolved{}, fmt.Errorf("accounting.prices_file is user-only")
			}
			if _, ok := values["extensions.file"]; ok {
				return Resolved{}, fmt.Errorf("extensions.file is user-only; project configuration cannot activate external code")
			}
			if _, ok := values["base_url"]; ok {
				return Resolved{}, fmt.Errorf("base_url is user-only; project endpoints cannot receive user credentials")
			}
			if _, ok := values["credentials.store"]; ok {
				return Resolved{}, fmt.Errorf("credentials.store is user-only; remove it from project configuration")
			}
			if _, ok := values["ui.theme"]; ok {
				return Resolved{}, fmt.Errorf("ui.theme is user-only; choose a terminal theme in user configuration")
			}
		}
		if err := result.apply(values, layer.source, fields); err != nil {
			return Resolved{}, err
		}
	}
	if opts.Env != nil {
		environment := make(map[string]string)
		for key := range fields {
			if value, ok := opts.Env(EnvironmentKey(key)); ok {
				environment[key] = value
			}
		}
		if err := result.apply(environment, "environment", fields); err != nil {
			return Resolved{}, err
		}
	}
	if err := result.apply(opts.Flags, "flag", fields); err != nil {
		return Resolved{}, err
	}
	providers, err := LoadCustomProviders(opts.Paths.ProvidersConfig, opts.Paths.ConfigDir)
	if err != nil {
		return Resolved{}, err
	}
	result.CustomProviders = providers
	return result, nil
}

func (r Resolved) String(key string) string {
	value, _ := r.Values[key].Value.(string)
	return value
}

func (r Resolved) Int(key string) int {
	value, _ := r.Values[key].Value.(int)
	return value
}

func (r Resolved) Bool(key string) bool {
	value, _ := r.Values[key].Value.(bool)
	return value
}

func (r Resolved) apply(values map[string]string, source string, fields map[string]Field) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		field, ok := fields[key]
		if !ok {
			return fmt.Errorf("unknown configuration key (source: %s); see config schema", source)
		}
		value, err := field.parse(values[key])
		if err != nil {
			return fmt.Errorf("%s configuration: %w", source, err)
		}
		r.Values[key] = Value{value, source}
	}
	return nil
}

func readTOML(path string, fields map[string]Field) (map[string]string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open configuration %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	const maxConfigBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil || len(data) > maxConfigBytes {
		return nil, fmt.Errorf("cannot read configuration %s (limit: 1 MiB)", path)
	}
	var tree map[string]any
	if err := toml.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("invalid TOML in %s; values omitted", path)
	}
	values := make(map[string]string)
	if err := flatten("", tree, fields, values); err != nil {
		return nil, fmt.Errorf("configuration %s: %w", path, err)
	}
	return values, nil
}

func readConfigFile(path string, fields map[string]Field) (map[string]string, error) {
	if filepath.Ext(path) != ".json" {
		return readTOML(path, fields)
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open settings %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	if err := platform.Check(filepath.Dir(path), true); err != nil {
		return nil, fmt.Errorf("settings directory permissions: %w", err)
	}
	if err := platform.Check(path, false); err != nil {
		return nil, fmt.Errorf("settings file permissions: %w", err)
	}
	const maxConfigBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil || len(data) > maxConfigBytes {
		return nil, fmt.Errorf("cannot read settings %s (limit: 1 MiB)", path)
	}
	var tree map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&tree); err != nil || tree == nil {
		return nil, fmt.Errorf("invalid JSON settings in %s; values omitted", path)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("invalid JSON settings in %s; values omitted", path)
	}
	values := make(map[string]string)
	if err := flattenJSON("", tree, fields, values); err != nil {
		return nil, fmt.Errorf("settings %s: %w", path, err)
	}
	return values, nil
}

func flatten(prefix string, tree map[string]any, fields map[string]Field, values map[string]string) error {
	for key, value := range tree {
		name := key
		if prefix != "" {
			name = prefix + "." + key
		}
		if children, ok := value.(map[string]any); ok {
			// Empty and unknown tables are errors too.
			known := false
			for field := range fields {
				if len(field) > len(name) && field[:len(name)+1] == name+"." {
					known = true
				}
			}
			if !known {
				return fmt.Errorf("unknown configuration table; see config schema")
			}
			if err := flatten(name, children, fields, values); err != nil {
				return err
			}
			continue
		}
		field, ok := fields[name]
		if !ok {
			return fmt.Errorf("unknown configuration key; see config schema (secrets belong in auth)")
		}
		switch v := value.(type) {
		case string:
			if field.Kind != "string" && field.Kind != "duration" {
				return fmt.Errorf("wrong TOML type for %s", name)
			}
			values[name] = v
		case bool:
			if field.Kind != "bool" {
				return fmt.Errorf("wrong TOML type for %s", name)
			}
			values[name] = strconv.FormatBool(v)
		case int64:
			if field.Kind != "int" {
				return fmt.Errorf("wrong TOML type for %s", name)
			}
			values[name] = strconv.FormatInt(v, 10)
		case json.Number:
			if field.Kind != "int" {
				return fmt.Errorf("wrong JSON type for %s", name)
			}
			if _, err := strconv.ParseInt(string(v), 10, 64); err != nil {
				return fmt.Errorf("wrong JSON type for %s", name)
			}
			values[name] = string(v)
		default:
			return fmt.Errorf("wrong TOML type for %s", name)
		}
	}
	return nil
}

func flattenJSON(prefix string, tree map[string]any, fields map[string]Field, values map[string]string) error {
	for key, raw := range tree {
		name := key
		if prefix != "" {
			name = prefix + "." + key
		}
		if children, ok := raw.(map[string]any); ok {
			known := false
			for field := range fields {
				if len(field) > len(name) && field[:len(name)+1] == name+"." {
					known = true
					break
				}
			}
			if !known {
				return fmt.Errorf("unknown configuration table; see config schema")
			}
			if err := flattenJSON(name, children, fields, values); err != nil {
				return err
			}
			continue
		}
		field, ok := fields[name]
		if !ok {
			return fmt.Errorf("unknown configuration key; see config schema (secrets belong in auth)")
		}
		switch value := raw.(type) {
		case string:
			if field.Kind != "string" && field.Kind != "duration" {
				return fmt.Errorf("wrong JSON type for %s", name)
			}
			values[name] = value
		case bool:
			if field.Kind != "bool" {
				return fmt.Errorf("wrong JSON type for %s", name)
			}
			values[name] = strconv.FormatBool(value)
		case json.Number:
			if field.Kind != "int" {
				return fmt.Errorf("wrong JSON type for %s", name)
			}
			if _, err := strconv.ParseInt(string(value), 10, 64); err != nil {
				return fmt.Errorf("wrong JSON type for %s", name)
			}
			values[name] = string(value)
		default:
			return fmt.Errorf("wrong JSON type for %s", name)
		}
	}
	return nil
}

func policyRank(policy string) int {
	switch policy {
	case "read-only":
		return 0
	case "ask":
		return 1
	case "trusted":
		return 2
	}
	return -1
}
