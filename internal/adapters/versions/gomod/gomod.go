// Package gomod checks and updates Go module requirements through the Go tool.
package gomod

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Module struct {
	Path     string   `json:"path"`
	Version  string   `json:"version,omitempty"`
	Main     bool     `json:"main,omitempty"`
	Indirect bool     `json:"indirect,omitempty"`
	Update   *Version `json:"update,omitempty"`
	Replace  *Version `json:"replace,omitempty"`
}

type Version struct {
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
}

type Report struct {
	Directory string   `json:"directory"`
	Modules   []Module `json:"modules"`
}

var (
	ErrNoModule  = errors.New("no Go module found")
	ErrGoMissing = errors.New("cannot find the Go executable")
)

func Check(ctx context.Context, directory string) (Report, error) {
	if err := requireModule(directory); err != nil {
		return Report{}, err
	}
	modfile, cleanup, err := readonlyModFile(directory)
	if err != nil {
		return Report{}, err
	}
	defer cleanup()
	output, err := run(ctx, directory, "list", "-modfile="+modfile, "-mod=readonly", "-m", "-u", "-json", "all")
	if err != nil {
		return Report{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	report := Report{Directory: directory}
	for {
		var module Module
		err := decoder.Decode(&module)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Report{}, fmt.Errorf("decode Go module list: %w", err)
		}
		if module.Path != "" {
			report.Modules = append(report.Modules, module)
		}
	}
	if len(report.Modules) == 0 {
		return Report{}, errors.New("module list returned by Go is empty")
	}
	return report, nil
}

// Update upgrades dependencies in the selected Go module. Empty selectors or
// "all" upgrade all reachable dependencies within their existing major paths;
// "patch" restricts that operation to patch releases. Explicit module selectors
// accept module or module@version and default to @latest.
func Update(ctx context.Context, directory string, selectors []string) (Report, error) {
	if err := requireModule(directory); err != nil {
		return Report{}, err
	}
	args, err := updateArgs(selectors)
	if err != nil {
		return Report{}, err
	}
	backup, err := backupModuleFiles(directory)
	if err != nil {
		return Report{}, err
	}
	if _, err := run(ctx, directory, args...); err != nil {
		return Report{}, errors.Join(err, backup.restore())
	}
	if _, err := run(ctx, directory, "mod", "tidy"); err != nil {
		return Report{}, errors.Join(err, backup.restore())
	}
	report, err := Check(ctx, directory)
	if err != nil {
		return Report{}, errors.Join(err, backup.restore())
	}
	return report, nil
}

func updateArgs(selectors []string) ([]string, error) {
	if len(selectors) == 0 || len(selectors) == 1 && selectors[0] == "all" {
		return []string{"get", "-u", "./..."}, nil
	}
	if len(selectors) == 1 && selectors[0] == "patch" {
		return []string{"get", "-u=patch", "./..."}, nil
	}
	args := []string{"get"}
	for _, selector := range selectors {
		if selector == "" || strings.HasPrefix(selector, "-") || strings.ContainsAny(selector, " \t\r\n") || strings.Count(selector, "@") > 1 {
			return nil, fmt.Errorf("invalid Go module selector %q", selector)
		}
		if strings.HasSuffix(selector, "@") || selector == "@latest" || strings.HasSuffix(selector, "@none") {
			return nil, fmt.Errorf("invalid or removing Go module selector %q", selector)
		}
		if !strings.Contains(selector, "@") {
			selector += "@latest"
		}
		args = append(args, selector)
	}
	return args, nil
}

func requireModule(directory string) error {
	info, err := os.Lstat(filepath.Join(directory, "go.mod"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w in %s; dependency version commands currently support Go modules", ErrNoModule, directory)
		}
		return fmt.Errorf("inspect go.mod: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("go.mod is not a regular file")
	}
	return nil
}

func run(ctx context.Context, directory string, args ...string) ([]byte, error) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("%w: install Go to check or update Go module versions", ErrGoMissing)
	}
	command := exec.CommandContext(ctx, goBinary, args...)
	command.Dir = directory
	command.Env = setEnv(setEnv(os.Environ(), "GOTOOLCHAIN", "local"), "GOWORK", "off")
	var output, diagnostic bytes.Buffer
	command.Stdout = &output
	command.Stderr = &diagnostic
	err = command.Run()
	if err != nil {
		message := strings.TrimSpace(diagnostic.String())
		if message == "" {
			message = strings.TrimSpace(output.String())
		}
		if len(message) > 4000 {
			message = message[:4000] + "…"
		}
		return nil, fmt.Errorf("go %s failed: %s: %w", strings.Join(args, " "), message, err)
	}
	return output.Bytes(), nil
}

func readonlyModFile(directory string) (string, func(), error) {
	contents, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	if err != nil {
		return "", nil, fmt.Errorf("read go.mod for a read-only version check: %w", err)
	}
	file, err := os.CreateTemp("", "opera-version-check-*.mod")
	if err != nil {
		return "", nil, fmt.Errorf("stage a read-only go.mod snapshot: %w", err)
	}
	modfile := file.Name()
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		_ = os.Remove(modfile)
		return "", nil, fmt.Errorf("write the go.mod snapshot: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(modfile)
		return "", nil, fmt.Errorf("close the go.mod snapshot: %w", err)
	}
	sumfile := strings.TrimSuffix(modfile, ".mod") + ".sum"
	if sum, err := os.ReadFile(filepath.Join(directory, "go.sum")); err == nil {
		if err := os.WriteFile(sumfile, sum, 0600); err != nil {
			_ = os.Remove(modfile)
			_ = os.Remove(sumfile)
			return "", nil, fmt.Errorf("write the go.sum snapshot: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(modfile)
		return "", nil, fmt.Errorf("read go.sum for a read-only version check: %w", err)
	}
	return modfile, func() {
		_ = os.Remove(modfile)
		_ = os.Remove(sumfile)
	}, nil
}

func setEnv(environment []string, key, value string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, prefix+value)
}

type fileBackup struct {
	directory string
	files     map[string]fileState
}

type fileState struct {
	contents []byte
	mode     os.FileMode
	exists   bool
}

func backupModuleFiles(directory string) (fileBackup, error) {
	backup := fileBackup{directory: directory, files: make(map[string]fileState, 2)}
	for _, name := range []string{"go.mod", "go.sum"} {
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			backup.files[name] = fileState{}
			continue
		}
		if err != nil {
			return fileBackup{}, fmt.Errorf("inspect %s before update: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return fileBackup{}, fmt.Errorf("refusing to update non-regular %s", name)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return fileBackup{}, fmt.Errorf("save %s before update: %w", name, err)
		}
		backup.files[name] = fileState{contents: contents, mode: info.Mode().Perm(), exists: true}
	}
	return backup, nil
}

func (backup fileBackup) restore() error {
	var restoreErrors []error
	for name, state := range backup.files {
		path := filepath.Join(backup.directory, name)
		if !state.exists {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				restoreErrors = append(restoreErrors, fmt.Errorf("remove new %s after failed update: %w", name, err))
			}
			continue
		}
		if current, err := os.ReadFile(path); err == nil && bytes.Equal(current, state.contents) {
			if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm() == state.mode {
				continue
			}
		}
		file, err := os.CreateTemp(backup.directory, ".opera-version-rollback-*")
		if err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s after failed update: %w", name, err))
			continue
		}
		temporary := file.Name()
		_, writeErr := file.Write(state.contents)
		chmodErr := file.Chmod(state.mode)
		closeErr := file.Close()
		if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
			_ = os.Remove(temporary)
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s after failed update: %w", name, err))
			continue
		}
		if err := os.Rename(temporary, path); err != nil {
			_ = os.Remove(temporary)
			restoreErrors = append(restoreErrors, fmt.Errorf("replace %s with saved version: %w", name, err))
		}
	}
	return errors.Join(restoreErrors...)
}
