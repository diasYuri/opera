package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/opera/internal/ui"
)

type accessibleLoginEvent struct {
	notice     execution.LoginNotice
	err        error
	models     []string
	catalogErr error
	done       bool
}

func accessibleLogin(ctx context.Context, auth execution.Authenticator, chooser ui.ModelChooser, id string) <-chan accessibleLoginEvent {
	events := make(chan accessibleLoginEvent, 1)
	go func() {
		defer close(events)
		err := auth.Login(ctx, id, func(notice execution.LoginNotice) error {
			select {
			case events <- accessibleLoginEvent{notice: notice}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		result := accessibleLoginEvent{done: true, err: err}
		if err == nil && chooser != nil {
			catalogCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			result.models, result.catalogErr = chooser.ModelChoices(catalogCtx)
			cancel()
		}
		select {
		case events <- result:
		case <-ctx.Done():
		}
	}()
	return events
}

func menuSelection(text string, size int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	return n - 1, err == nil && n >= 1 && n <= size
}

func loginMenuText(providers []execution.LoginProvider) string {
	var text strings.Builder
	text.WriteString("Connect a provider\n")
	for i, p := range providers {
		fmt.Fprintf(&text, "%d. %s — %s\n", i+1, p.Name, p.Description)
	}
	text.WriteString("Enter a number or provider name; /cancel goes back.\nYou> ")
	return text.String()
}

func loginResultText(event accessibleLoginEvent) string {
	if event.err != nil {
		if errors.Is(event.err, context.Canceled) {
			return "Login cancelled.\nYou> "
		}
		return "Login failed: " + event.err.Error() + "\nUse /login to try again.\nYou> "
	}
	if event.catalogErr != nil {
		return "Signed in. Model catalog unavailable: " + event.catalogErr.Error() + "\nUse /model to try again.\nYou> "
	}
	var text strings.Builder
	text.WriteString("Signed in. Choose a model:\n")
	for i, m := range event.models {
		fmt.Fprintf(&text, "%d. %s\n", i+1, m)
	}
	text.WriteString("Enter a number or /model <provider> <model>; /cancel goes back.\nYou> ")
	return text.String()
}

func loginPromptText(prompt *execution.LoginPrompt) string {
	var text strings.Builder
	text.WriteString(prompt.Title + "\n")
	if prompt.Message != "" {
		text.WriteString(prompt.Message + "\n")
	}
	if prompt.Kind == "secret" {
		text.WriteString("Secret input is unavailable in accessible mode. Set it through opera auth set from piped stdin.\nYou> ")
		return text.String()
	}
	if len(prompt.Options) == 0 {
		text.WriteString("No choices are available.\nYou> ")
		return text.String()
	}
	for i, option := range prompt.Options {
		fmt.Fprintf(&text, "%d. %s — %s\n", i+1, option.Name, option.Description)
	}
	text.WriteString("Enter a number or /cancel.\nYou> ")
	return text.String()
}
