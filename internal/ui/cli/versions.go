package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/diasYuri/opera/internal/adapters/updates"
	"github.com/diasYuri/opera/internal/adapters/versions/gomod"
	"github.com/diasYuri/opera/internal/modules/versions"
)

type updateReport struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	ReleaseURL      string `json:"release_url,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Applied         bool   `json:"applied"`
	Message         string `json:"message"`
}

type dependencyReport struct {
	Action   string       `json:"action"`
	Selected []string     `json:"selected,omitempty"`
	Report   gomod.Report `json:"report"`
}

func (a App) versionsCommand(command string, opts options, jsonl bool) int {
	if command == "update" {
		return a.updateOpera(opts.args[1:], jsonl)
	}
	return a.updateDependencies(opts, jsonl)
}

func (a App) updateOpera(args []string, jsonl bool) int {
	if len(args) > 1 || len(args) == 1 && args[0] != "check" && args[0] != "apply" {
		return a.fail(ExitUsage, errors.New("usage: opera update [check|apply]"), jsonl)
	}
	action := "check"
	if len(args) == 1 {
		action = args[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	release, err := updates.NewClient().Latest(ctx)
	if err != nil {
		return a.fail(ExitFailure, err, jsonl)
	}
	comparison, err := versions.Compare(a.Version, release.Version)
	if err != nil {
		return a.fail(ExitFailure, fmt.Errorf("compare Opera versions: %w", err), jsonl)
	}
	available := comparison < 0 || strings.Contains(a.Version, "-local") || strings.Contains(a.Version, "-dev")
	report := updateReport{
		Current: a.Version, Latest: release.Version, ReleaseURL: release.URL,
		UpdateAvailable: available,
	}
	if !available {
		report.Message = "Opera is up to date."
	} else if action == "check" {
		report.Message = "A newer Opera release is available; run `opera update apply` to install it."
	} else {
		if strings.Contains(a.Version, "-local") || strings.Contains(a.Version, "-dev") {
			return a.fail(ExitFailure, errors.New("development builds cannot replace their executable; use the installer or a release package"), jsonl)
		}
		if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
			return a.fail(ExitFailure, fmt.Errorf("in-place update is supported on macOS and Linux, not %s", runtime.GOOS), jsonl)
		}
		binary, err := updates.NewClient().Binary(ctx, release)
		if err != nil {
			return a.fail(ExitFailure, err, jsonl)
		}
		if err := updates.ReplaceCurrent(binary); err != nil {
			return a.fail(ExitFailure, err, jsonl)
		}
		report.Applied = true
		report.Message = fmt.Sprintf("Opera %s installed. Start a new Opera process to use it.", release.Version)
	}
	if jsonl {
		return a.emit("versions.opera", report)
	}
	return a.writeText(fmt.Sprintf("%s\nCurrent: %s\nLatest: %s\nRelease: %s\n", report.Message, report.Current, report.Latest, report.ReleaseURL))
}

func (a App) updateDependencies(opts options, jsonl bool) int {
	args := opts.args[1:]
	if len(args) == 0 {
		args = []string{"check"}
	}
	action := args[0]
	if action != "check" && action != "update" {
		return a.fail(ExitUsage, errors.New("usage: opera deps [check|update [all|patch|module[@version]...]]"), jsonl)
	}
	selectors := args[1:]
	if action == "check" && len(selectors) > 0 {
		return a.fail(ExitUsage, errors.New("usage: opera deps check"), jsonl)
	}
	if action == "update" && len(selectors) > 1 && (selectors[0] == "all" || selectors[0] == "patch") {
		return a.fail(ExitUsage, errors.New("use `all` or `patch` alone, or pass one or more module selectors"), jsonl)
	}
	directory := opts.workspace
	if directory == "" {
		var err error
		directory, err = a.Getwd()
		if err != nil {
			return a.fail(ExitFailure, fmt.Errorf("resolve project directory: %w", err), jsonl)
		}
	}
	var err error
	directory, err = filepath.Abs(directory)
	if err != nil {
		return a.fail(ExitUsage, fmt.Errorf("resolve project directory: %w", err), jsonl)
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("path is not a directory")
		}
		return a.fail(ExitUsage, fmt.Errorf("invalid project directory: %w", err), jsonl)
	}
	deadline := 2 * time.Minute
	if action == "update" {
		deadline = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	var report gomod.Report
	if action == "check" {
		report, err = gomod.Check(ctx, directory)
	} else {
		report, err = gomod.Update(ctx, directory, selectors)
	}
	if err != nil {
		if errors.Is(err, gomod.ErrNoModule) || errors.Is(err, gomod.ErrGoMissing) {
			return a.fail(ExitUsage, err, jsonl)
		}
		return a.fail(ExitFailure, err, jsonl)
	}
	result := dependencyReport{Action: action, Selected: selectors, Report: report}
	if jsonl {
		return a.emit("versions.dependencies", result)
	}
	return a.writeText(renderDependencyReport(result))
}

func renderDependencyReport(result dependencyReport) string {
	var out strings.Builder
	if result.Action == "update" {
		fmt.Fprintf(&out, "Updated Go module requirements in %s.\n", result.Report.Directory)
	} else {
		fmt.Fprintf(&out, "Go module version check for %s\n", result.Report.Directory)
	}
	updates := 0
	for _, module := range result.Report.Modules {
		if module.Main || module.Update == nil || module.Update.Version == "" || module.Update.Version == module.Version {
			continue
		}
		updates++
		kind := "direct"
		if module.Indirect {
			kind = "indirect"
		}
		fmt.Fprintf(&out, "  %s: %s → %s (%s)\n", module.Path, module.Version, module.Update.Version, kind)
	}
	if updates == 0 {
		out.WriteString("All Go module dependencies are current.\n")
	} else if result.Action == "update" {
		fmt.Fprintf(&out, "%d dependency update(s) remain available, usually major-version or module-path changes.\n", updates)
	} else {
		fmt.Fprintf(&out, "%d dependency update(s) available. Use `opera deps update` to upgrade within current major versions.\n", updates)
	}
	return out.String()
}
