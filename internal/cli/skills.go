package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/diasYuri/opera/internal/adapters/skills/filesystem"
	"github.com/diasYuri/opera/internal/modules/configuration"
)

func (a App) skills(args []string, cfg config.Resolved, jsonl bool) int {
	if len(args) == 0 {
		args = []string{"list"}
	}
	if len(args) > 2 {
		return a.fail(ExitUsage, errors.New("usage: opera skills [list|check|show <name>]"), jsonl)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return a.fail(ExitFailure, fmt.Errorf("resolve user home: %w", err), jsonl)
	}
	catalog := skills.Discover(cfg.Paths.Workspace, home)
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return a.fail(ExitUsage, errors.New("usage: opera skills list"), jsonl)
		}
		if jsonl {
			return a.emit("skills.catalog", catalog)
		}
		for _, skill := range catalog.Skills {
			if _, err := fmt.Fprintf(a.Out, "$%s (%s) — %s\n", skill.Name, skill.Source, skill.Description); err != nil {
				return ExitFailure
			}
		}
		for _, issue := range catalog.Issues {
			if _, err := fmt.Fprintf(a.Err, "invalid skill %s (%s): %s\n", issue.Name, issue.Source, issue.Error); err != nil {
				return ExitFailure
			}
		}
		return ExitOK
	case "check":
		if len(args) != 1 {
			return a.fail(ExitUsage, errors.New("usage: opera skills check"), jsonl)
		}
		if jsonl {
			return a.emit("skills.check", catalog)
		}
		if len(catalog.Issues) > 0 {
			for _, issue := range catalog.Issues {
				_, _ = fmt.Fprintf(a.Err, "invalid skill %s (%s): %s\n", issue.Name, issue.Source, issue.Error)
			}
			return ExitFailure
		}
		return a.writeText(fmt.Sprintf("%d skill(s) valid\n", len(catalog.Skills)))
	case "show":
		if len(args) != 2 {
			return a.fail(ExitUsage, errors.New("usage: opera skills show <name>"), jsonl)
		}
		skill, ok := catalog.Find(args[1])
		if !ok {
			return a.fail(ExitUsage, fmt.Errorf("unknown or invalid skill %q", args[1]), jsonl)
		}
		if jsonl {
			return a.emit("skills.skill", skill)
		}
		return a.writeText(fmt.Sprintf("%s (%s)\n%s\n\n%s\n", skill.Name, skill.Source, skill.Description, skill.Body))
	default:
		return a.fail(ExitUsage, errors.New("usage: opera skills [list|check|show <name>]"), jsonl)
	}
}
