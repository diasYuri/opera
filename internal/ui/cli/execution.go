package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	ui "github.com/diasYuri/opera/internal/ui/tui"
)

func (a App) execution(command string, opts options, cfg config.Resolved, jsonl bool) (code int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := execution.Request{Management: opts.management}
	if (opts.approve != "" || opts.deny != "") && command != "resume" && command != "retry" {
		return a.fail(2, errors.New("approval flags require resume"), jsonl)
	}
	switch command {
	case "run", "worker":
		if len(opts.args) != 2 || strings.TrimSpace(opts.args[1]) == "" {
			return a.fail(2, errors.New("usage: opera run <prompt>"), jsonl)
		}
		request.Prompt = opts.args[1]
		request.Worker = command == "worker"
		request.ContinueFrom = opts.continueFrom
	case "resume", "retry":
		if len(opts.args) < 2 || len(opts.args) > 3 {
			return a.fail(2, errors.New("usage: opera resume <run-id> [--approve <approval-id>|--deny <approval-id>]"), jsonl)
		}
		request.ID = opts.args[1]
		if len(opts.args) == 3 {
			request.Prompt = opts.args[2]
		}
		request.Approve = opts.approve
		request.Deny = opts.deny
	case "chat", "onboarding":
		if len(opts.args) != 1 {
			return a.fail(2, errors.New("usage: opera chat"), jsonl)
		}
		if !a.IsTTY {
			return a.fail(2, errors.New("chat requires a TTY; use opera run <prompt> --format jsonl"), jsonl)
		}
	case "reconcile":
		if len(opts.args) != 4 {
			return a.fail(2, errors.New("usage: opera reconcile <run-id> <operation-key> <observed-result>"), jsonl)
		}
		request.ID = opts.args[1]
	}
	if command == "onboarding" || command != "chat" && !opts.management && request.ID == "" && (cfg.String("provider") == "" || cfg.String("model") == "" || cfg.Values["policy"].Source == "default") {
		if !a.IsTTY || jsonl {
			return a.fail(2, errors.New("choose provider, model and workspace policy explicitly via flags, environment or configuration; no forms are opened without a TTY"), jsonl)
		}
		var err error
		cfg, err = a.onboard(ctx, cfg)
		if err != nil {
			return a.fail(1, err, jsonl)
		}
		if command == "onboarding" {
			return a.writeText("Workspace selection saved.\n")
		}
	}
	format, err := readResponseFormat(opts.responseFormat, opts.responseSchema)
	if err != nil {
		return a.fail(2, err, jsonl)
	}
	harnessOpts := core.Options{ProviderCatalog: a.ProviderCatalog, ResponseFormat: format, Management: opts.management, Offline: opts.offline, Secret: a.extensionSecret(cfg), Thinking: cfg.String("thinking"), ContextPaths: opts.attachments, ResumeID: request.ID, Credential: a.credentialLookup(cfg)}
	if command == "chat" && !jsonl {
		return a.interactiveChat(ctx, cfg, harnessOpts, request)
	}
	runtime, err := core.Open(ctx, cfg, harnessOpts)
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	cleanupRuntime := runtime.Close
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if err := cleanupRuntime(cleanup); err != nil {
			if code == 0 {
				code = ExitFailure
			}
			a.fail(ExitFailure, fmt.Errorf("cleanup failed: %w", err), jsonl)
		}
	}()
	if command == "reconcile" {
		metadata, err := runtime.Service.Metadata(ctx, request.ID)
		if err != nil || metadata.Workspace != cfg.Paths.Workspace {
			return a.fail(2, errors.New("run does not belong to this workspace"), jsonl)
		}
		operations, err := runtime.Service.Store.Operations(ctx, request.ID)
		if err != nil {
			return a.fail(1, err, jsonl)
		}
		found := false
		for _, operation := range operations {
			if operation.IdempotencyKey == opts.args[2] {
				found = true
			}
		}
		if !found {
			return a.fail(2, errors.New("operation does not belong to this run"), jsonl)
		}
		if err := execution.RecordObservedOperation(ctx, runtime.Service.Store, opts.args[2], opts.args[3]); err != nil {
			return a.fail(1, err, jsonl)
		}
		if jsonl {
			return a.emit("operation.reconciled", map[string]string{"run_id": request.ID, "operation_key": opts.args[2]})
		}
		return a.writeText("Observed operation result recorded. Resume explicitly; no effect was repeated.\n")
	}
	if (command == "chat" || command == "resume" && a.IsTTY) && !jsonl && !cfg.Bool("ui.accessible") {
		session := newChatSession(a, cfg, harnessOpts, runtime)
		cleanupRuntime = session.Close
		code, err := ui.Chat(ctx, session, a.In, a.Out, request, cfg.Bool("ui.no_motion"))
		if err != nil {
			return a.fail(1, err, false)
		}
		return code
	}
	if command == "chat" && jsonl {
		return a.fail(2, errors.New("JSONL chat input is unsupported; use opera run <prompt> --format=jsonl"), true)
	}
	if command == "chat" {
		return a.accessibleChat(ctx, runtime)
	}
	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	lastAgent := ""
	result, err := runtime.Service.Execute(signalCtx, request, func(event execution.Event) error {
		if jsonl {
			return json.NewEncoder(a.Out).Encode(event)
		}
		if event.Text != "" {
			agent := ""
			if event.ParentRunID != "" {
				agent = event.RunID
			}
			if agent != lastAgent {
				label := "Opera"
				if agent != "" {
					label = "Agent " + agent
				}
				if _, err := fmt.Fprintln(a.Out, "\n"+label); err != nil {
					return err
				}
				lastAgent = agent
			}
			_, err := fmt.Fprint(a.Out, event.Text)
			return err
		}
		return nil
	})
	if jsonl {
		if writeErr := json.NewEncoder(a.Out).Encode(struct {
			SchemaVersion int              `json:"schema_version"`
			Type          string           `json:"type"`
			Data          execution.Result `json:"data"`
		}{1, "execution.result", result}); writeErr != nil {
			return 1
		}
	} else {
		if _, writeErr := fmt.Fprintf(a.Out, "\n%s · run %s · checkpoint %d\n", result.State, result.ID, result.Checkpoint); writeErr != nil {
			return 1
		}
		if result.Approval != nil {
			if _, writeErr := fmt.Fprintf(a.Out, "Agent: %s · task %s\nTool: %s\nArguments: %s\n%s\nResume: opera resume %s --approve %s\nDeny: opera resume %s --deny %s\n", result.Approval.Agent, result.Approval.TaskID, result.Approval.Tool, result.Approval.Arguments, result.Approval.Diff, result.ID, result.Approval.ID, result.ID, result.Approval.ID); writeErr != nil {
				return 1
			}
		}
	}
	if err != nil {
		return a.fail(result.ExitCode, err, jsonl)
	}
	return result.ExitCode
}
