package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/execution"
)

type accessibleActivityChat struct{ *chatSession }

func (accessibleActivityChat) Start(context.Context, execution.Request) *execution.Execution {
	events := []execution.Event{
		{Type: "message_delta", Text: "Intermediate comment"},
		{Type: "message_completed", Data: []byte(`{"message":{"role":"assistant","tool_calls":[{}]}}`)},
		{Type: "tool_started"},
		{Type: "message_started"},
		{Type: "message_delta", Text: "\x1b[2JFinal answer"},
		{Type: "message_completed", Data: []byte(`{"message":{"role":"assistant"}}`)},
	}
	updates := make(chan execution.Update, len(events)+1)
	for _, event := range events {
		updates <- execution.Update{Event: &event}
	}
	updates <- execution.Update{Result: &execution.Result{ID: "fixture", State: "completed"}}
	close(updates)
	return execution.NewExecution("fixture", updates, func() {})
}

func TestAccessibleActivityDisplayUsesConfirmedResponses(t *testing.T) {
	for _, mode := range []string{"full", "auto", "hidden"} {
		t.Run(mode, func(t *testing.T) {
			var out strings.Builder
			app := App{In: strings.NewReader("/activity " + mode + "\nTest prompt\n"), Out: &out}
			cfg := sessionConfig(t)
			chat := accessibleActivityChat{newChatSession(app, cfg, core.Options{}, nil)}
			if code := app.accessibleChat(context.Background(), "full", chat); code != 0 {
				t.Fatalf("accessible chat exited %d", code)
			}
			text := out.String()
			if strings.Count(text, "Final answer") != 1 || strings.ContainsRune(text, '\x1b') {
				t.Fatalf("expected one plain-text answer in %q", text)
			}
			if strings.Contains(text, "Intermediate comment") != (mode != "hidden") {
				t.Fatalf("%s showed the wrong commentary in %q", mode, text)
			}
			if !strings.Contains(text, "Activity display set to "+mode+" for this workspace.") {
				t.Fatalf("configuration was not saved: %q", text)
			}
		})
	}
}
