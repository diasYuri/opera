package demo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/harness"
	taskmanager "github.com/diasYuri/y/pkg/extensions/task-manager"
	"github.com/diasYuri/y/pkg/providers"
)

type Evidence struct {
	Tasks     int    `json:"tasks"`
	Mode      string `json:"mode"`
	RunID     string `json:"run_id"`
	Approvals int    `json:"approvals"`
	Denials   int    `json:"denials"`
	Children  int    `json:"children"`
	Requests  int64  `json:"provider_requests"`
	Verified  bool   `json:"verified"`
}

func Run(ctx context.Context, cfg config.Resolved, mode string, p Provider, emit func(app.Event) error) (e Evidence, err error) {
	e.Mode = mode
	if mode != "chat" && mode != "run" && mode != "worker" {
		return e, errors.New("demo mode must be chat, run or worker")
	}
	if err := os.MkdirAll(cfg.Paths.Workspace, 0700); err != nil {
		return e, err
	}
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "AGENTS.md"), []byte("Verify the demo change. Do not write denied.txt."), 0600); err != nil {
		return e, err
	}
	open := func() (*harness.Runtime, error) {
		return harness.Open(ctx, cfg, harness.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return p, nil }})
	}
	r, err := open()
	if err != nil {
		return e, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if r != nil {
			err = errors.Join(err, r.Close(cleanup))
		}
	}()
	request := app.Request{Prompt: "Implement and verify the demo change", Worker: mode == "worker"}
	var result app.Result
	if mode == "chat" {
		execution := r.Start(ctx, request)
		for update := range execution.Updates {
			if update.Event != nil && emit != nil {
				if err := emit(*update.Event); err != nil {
					execution.Cancel()
					return e, err
				}
			}
			if update.Result != nil {
				result = *update.Result
				err = update.Error
			}
		}
	} else {
		result, err = r.Service.Execute(ctx, request, emit)
	}
	if err != nil {
		return e, err
	}
	for attempts := 0; result.ExitCode == 3 && attempts < 12; attempts++ {
		if result.Approval == nil {
			return e, errors.New("demo has no actionable pending approval")
		}
		e.RunID = result.ID
		next := app.Request{ID: result.ID, Approve: result.Approval.ID}
		if result.Approval.Tool == "write_file" && filepath.Base(result.Approval.Path) == "denied.txt" {
			next.Approve = ""
			next.Deny = result.Approval.ID
			e.Denials++
		} else {
			e.Approvals++
		}
		// Exercise explicit close/reopen handoff at every approval boundary.
		if err := r.Close(ctx); err != nil {
			return e, err
		}
		r, err = open()
		if err != nil {
			return e, err
		}
		result, err = r.Service.Execute(ctx, next, emit)
		if err != nil {
			return e, err
		}
	}
	e.RunID = result.ID
	if result.ExitCode != 0 {
		return e, errors.New("demo execution did not complete")
	}
	data, err := os.ReadFile(filepath.Join(cfg.Paths.Workspace, "approved.txt"))
	if err != nil || string(data) != "opera verified" {
		return e, errors.New("demo approved effect missing")
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.Workspace, "denied.txt")); !os.IsNotExist(err) {
		return e, errors.New("denied effect occurred")
	}
	view, err := r.Service.Inspect(ctx, result.ID)
	if err != nil {
		return e, err
	}
	e.Children = len(view.Children)
	tasks, err := r.Service.Work.Tasks(ctx, result.ID)
	if err != nil {
		return e, err
	}
	manager := taskmanager.New()
	if err := manager.Restore(tasks); err != nil {
		return e, err
	}
	e.Tasks = len(tasks.Tasks)
	if e.Tasks < 3 || !manager.AllComplete() {
		return e, errors.New("demo plan was not persisted and completed")
	}
	report, err := r.Service.Usage(ctx, "run", result.ID, "")
	if err != nil {
		return e, err
	}
	e.Requests = report.Totals.Total.Count
	if e.Denials != 1 || e.Approvals < 2 || e.Children != 1 || e.Requests < 5 {
		return e, errors.New("incomplete demo evidence")
	}
	e.Verified = true
	return e, nil
}
