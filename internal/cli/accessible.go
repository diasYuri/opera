package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/ui"
)

// Accessible chat is append-only and keeps input/control responsive while the
// same application execution streams. It never enters raw mode or redraws.
func (a App) accessibleChat(ctx context.Context, r interface {
	ui.Executor
	ui.Commander
}) int {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type input struct {
		text string
		err  error
		eof  bool
	}
	lines := make(chan input, 8)
	go func() {
		scanner := bufio.NewScanner(a.In)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			select {
			case lines <- input{text: scanner.Text()}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case lines <- input{err: scanner.Err(), eof: true}:
		case <-ctx.Done():
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	var loginPrompt *app.LoginPrompt
	var loginProviders []app.LoginProvider
	var modelMenu []string
	var loginCancel context.CancelFunc
	var loginEvents <-chan accessibleLoginEvent
	defer func() {
		if loginCancel != nil {
			loginCancel()
		}
	}()
	var execution *app.Execution
	var updates <-chan app.Update
	var pending *app.Result
	previous := ""
	mode := app.ModeDefault
	retryPrevious := false
	lastCode := 0
	closedInput, quitting, followUp := false, false, false
	multiline := false
	var buffer []string
	write := func(text string) bool { _, err := fmt.Fprint(a.Out, ansi.Strip(text)); return err == nil }
	start := func(req app.Request) { execution = r.Start(ctx, req); updates = execution.Updates; pending = nil }
	if !write("Append-only chat. /help lists commands; /multiline and /send compose multiple lines.\nYou> ") {
		return 1
	}
	for {
		select {
		case <-ctx.Done():
			if execution != nil {
				execution.Cancel()
			}
			return 130
		case <-signals:
			if loginCancel != nil {
				loginCancel()
				continue
			}
			if execution != nil {
				execution.Cancel()
				if !write("\nCancelling execution; session stays open.\n") {
					return 1
				}
			} else {
				return 0
			}
		case in := <-lines:
			if in.err != nil {
				return a.fail(1, in.err, false)
			}
			if in.eof {
				closedInput = true
				lines = nil
				if execution == nil && loginCancel == nil {
					if pending != nil {
						return 3
					}
					return lastCode
				}
				if a.IsTTY {
					quitting = true
					execution.Cancel()
				}
				continue
			}
			text := in.text
			if text == "/plan" || strings.HasPrefix(text, "/plan ") {
				if text != "/plan" {
					if !write("Use /plan without arguments.\nYou> ") {
						return 1
					}
					continue
				}
				if execution != nil || pending != nil {
					if !write("Resolve the active execution before changing modes.\nYou> ") {
						return 1
					}
					continue
				}
				if mode == app.ModePlan {
					mode = app.ModeDefault
				} else {
					mode = app.ModePlan
				}
				if !write("Agent mode: " + mode + ".\nYou> ") {
					return 1
				}
				continue
			}
			commandName, _, _ := strings.Cut(text, " ")
			if commandName == "/mode" {
				fields := strings.Fields(text)
				if execution != nil || pending != nil {
					if !write("Resolve the active execution before changing modes.\nYou> ") {
						return 1
					}
					continue
				}
				if len(fields) == 1 {
					var options []string
					for _, available := range app.AvailableModes() {
						options = append(options, available.ID+" — "+available.Description)
					}
					if !write("Available modes:\n" + strings.Join(options, "\n") + "\nUse /mode <name>.\nYou> ") {
						return 1
					}
					continue
				}
				if len(fields) != 2 {
					if !write("Use /mode [name].\nYou> ") {
						return 1
					}
					continue
				}
				selected, err := app.ResolveMode(fields[1])
				if err != nil {
					if !write(err.Error() + "\nYou> ") {
						return 1
					}
					continue
				}
				mode = selected.ID
				if !write("Agent mode: " + mode + ".\nYou> ") {
					return 1
				}
				continue
			}
			if text == "/quit" {
				if execution == nil {
					return 0
				}
				quitting = true
				execution.Cancel()
				continue
			}
			if loginPrompt != nil {
				if text == "/cancel" {
					loginCancel()
					loginPrompt = nil
					continue
				}
				selected, ok := menuSelection(text, len(loginPrompt.Options))
				if !ok {
					for i, option := range loginPrompt.Options {
						if strings.EqualFold(text, option.ID) {
							selected, ok = i, true
							break
						}
					}
				}
				if !ok {
					if !write("Choose a listed storage option or /cancel.\nYou> ") {
						return 1
					}
					continue
				}
				select {
				case loginPrompt.Response <- loginPrompt.Options[selected].ID:
				case <-ctx.Done():
					return 130
				}
				loginPrompt = nil
				if !write("Continuing sign-in…\n") {
					return 1
				}
				continue
			}
			if loginCancel != nil {
				if text == "/cancel" {
					loginCancel()
				} else if !write("Sign-in is in progress; /cancel stops it.\n") {
					return 1
				}
				continue
			}
			if text == "/login" {
				if execution != nil || pending != nil {
					if !write("Resolve the active execution before signing in.\n") {
						return 1
					}
					continue
				}
				auth, ok := r.(app.Authenticator)
				if !ok {
					if !write("Login is unavailable in this session.\n") {
						return 1
					}
					continue
				}
				loginProviders = auth.LoginProviders()
				modelMenu = nil
				if !write(loginMenuText(loginProviders)) {
					return 1
				}
				continue
			}
			if len(loginProviders) > 0 {
				if text == "/cancel" {
					loginProviders = nil
					if !write("You> ") {
						return 1
					}
					continue
				}
				selected, ok := menuSelection(text, len(loginProviders))
				if !ok {
					for i, p := range loginProviders {
						if strings.EqualFold(text, p.ID) || strings.EqualFold(text, p.Name) {
							selected, ok = i, true
							break
						}
					}
				}
				if !ok {
					if !write("Choose a listed provider or /cancel.\nYou> ") {
						return 1
					}
					continue
				}
				auth := r.(app.Authenticator)
				chooser, _ := r.(ui.ModelChooser)
				loginCtx, cancel := context.WithCancel(ctx)
				loginCancel = cancel
				loginEvents = accessibleLogin(loginCtx, auth, chooser, loginProviders[selected].ID)
				loginProviders = nil
				if !write("Connecting… /cancel stops sign-in.\n") {
					return 1
				}
				continue
			}
			if len(modelMenu) > 0 {
				if text == "/cancel" {
					modelMenu = nil
					if !write("You> ") {
						return 1
					}
					continue
				}
				if selected, ok := menuSelection(text, len(modelMenu)); ok {
					text = "/model " + modelMenu[selected]
				} else if !strings.HasPrefix(text, "/") {
					if !write("Choose a model number or /cancel.\nYou> ") {
						return 1
					}
					continue
				}
				modelMenu = nil
			}
			if text == "/multiline" {
				multiline = true
				buffer = nil
				if !write("Multiline input; /send submits, /cancel-input discards.\n") {
					return 1
				}
				continue
			}
			if multiline {
				if text == "/cancel-input" {
					multiline = false
					buffer = nil
					continue
				}
				if text != "/send" {
					buffer = append(buffer, text)
					continue
				}
				text = strings.Join(buffer, "\n")
				multiline = false
				buffer = nil
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			if pending != nil {
				req := app.Request{ID: pending.ID}
				switch text {
				case "y", "/approve":
					req.Approve = pending.Approval.ID
				case "n", "/deny":
					req.Deny = pending.Approval.ID
				default:
					if !write("Approval pending; use /approve or /deny.\n") {
						return 1
					}
					continue
				}
				start(req)
				continue
			}
			name, rest, _ := strings.Cut(text, " ")
			if name == "/steer" {
				if execution == nil || strings.TrimSpace(rest) == "" {
					if !write("Steering requires an active execution and a message.\n") {
						return 1
					}
					continue
				}
				if err := r.Steer(execution.ID, rest); err != nil {
					if !write(err.Error() + "\n") {
						return 1
					}
				}
				continue
			}
			if name == "/retry" {
				if execution != nil || previous == "" {
					if !write("Retry requires an idle previous run.\n") {
						return 1
					}
					continue
				}
				start(app.Request{ID: previous})
				continue
			}
			if name == "/new" || name == "/clear" || name == "/fork" {
				if text != name {
					if !write("Use the command without arguments.\nYou> ") {
						return 1
					}
					continue
				}
				lifecycle, ok := r.(ui.SessionLifecycle)
				if !ok {
					if !write("Session commands are unavailable in this chat.\nYou> ") {
						return 1
					}
					continue
				}
				if execution != nil || pending != nil {
					if !write("Resolve the active execution before changing sessions.\nYou> ") {
						return 1
					}
					continue
				}
				if name == "/fork" {
					if previous == "" {
						if !write("There is no completed session to fork.\nYou> ") {
							return 1
						}
						continue
					}
					branch, err := lifecycle.ForkSession(ctx, previous)
					if err != nil {
						if !write(err.Error() + "\nYou> ") {
							return 1
						}
						continue
					}
					previous, retryPrevious = branch, true
					if !write("Fork ready with a summary of the previous session.\nYou> ") {
						return 1
					}
					continue
				}
				if err := lifecycle.NewSession(ctx); err != nil {
					if !write(err.Error() + "\nYou> ") {
						return 1
					}
					continue
				}
				previous, retryPrevious = "", false
				if !write("Started a new session.\nYou> ") {
					return 1
				}
				continue
			}
			if strings.HasPrefix(text, "/") {
				if name == "/usage" || name == "/tasks" || name == "/agents" || name == "/compact" {
					id := previous
					if execution != nil {
						id = execution.ID
					}
					text = name + " " + id
				}
				value, err := r.Command(ctx, text)
				if err != nil {
					value = err.Error()
				}
				if !write(value + "\nYou> ") {
					return 1
				}
				continue
			}
			if execution != nil {
				if err := r.FollowUp(execution.ID, text); err != nil {
					if !write(err.Error() + "\n") {
						return 1
					}
				} else {
					followUp = true
				}
				continue
			}
			request := app.Request{Prompt: text, ContinueFrom: previous, Mode: mode}
			if retryPrevious && previous != "" {
				request.ID = previous
				request.ContinueFrom = ""
			}
			start(request)
		case event, ok := <-loginEvents:
			if !ok {
				loginEvents = nil
				loginPrompt = nil
				if loginCancel != nil {
					loginCancel()
					loginCancel = nil
					if !write("Login cancelled.\nYou> ") {
						return 1
					}
				}
				if closedInput {
					return lastCode
				}
				continue
			}
			if event.done {
				loginPrompt = nil
				loginEvents = nil
				loginCancel()
				loginCancel = nil
				modelMenu = event.models
				if !write(loginResultText(event)) {
					return 1
				}
				if event.err != nil && !errors.Is(event.err, context.Canceled) {
					lastCode = 1
				} else if event.err == nil {
					lastCode = 0
				}
				if closedInput {
					return lastCode
				}
			} else {
				if event.notice.Prompt != nil {
					loginPrompt = event.notice.Prompt
					if !write(loginPromptText(loginPrompt)) {
						return 1
					}
					continue
				}
				if event.notice.URL != "" && !write(event.notice.URL+"\n") {
					return 1
				}
				if event.notice.Message != "" && !write(event.notice.Message+"\n") {
					return 1
				}
			}
		case update, ok := <-updates:
			if !ok {
				updates = nil
				execution = nil
				if closedInput || quitting {
					return lastCode
				}
				continue
			}
			if update.Event != nil && update.Event.Text != "" {
				if !write(update.Event.Text) {
					execution.Cancel()
					return 1
				}
			}
			if update.Result == nil {
				continue
			}
			result := *update.Result
			updates = nil
			execution = nil
			if result.State != "" {
				previous = result.ID
				retryPrevious = result.ExitCode != 0 && result.Approval == nil
			}
			lastCode = result.ExitCode
			if !write(fmt.Sprintf("\n%s · run %s · checkpoint %d\n", result.State, result.ID, result.Checkpoint)) {
				return 1
			}
			if update.Error != nil {
				if !write(update.Error.Error() + "\n") {
					return 1
				}
			}
			if result.Approval != nil {
				pending = &result
				if !write(fmt.Sprintf("Agent: %s\nTool: %s\nArguments: %s\n%s\nResume: opera resume %s --approve %s\n", result.Approval.Agent, result.Approval.Tool, result.Approval.Arguments, result.Approval.Diff, result.ID, result.Approval.ID)) {
					return 1
				}
				if !a.IsTTY || closedInput {
					return 3
				}
				if !write("Use /approve or /deny (y/n).\n") {
					return 1
				}
			}
			if result.ExitCode == 0 && followUp {
				followUp = false
				start(app.Request{ContinueFrom: previous})
				continue
			}
			if quitting || closedInput {
				return lastCode
			}
			if !write("You> ") {
				return 1
			}
		}
	}
}
