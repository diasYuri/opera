package ui

import (
	"context"
	"github.com/diasYuri/opera/internal/app"
)

type terminalExecutor struct{}

func (terminalExecutor) Start(context.Context, app.Request) *app.Execution {
	updates := make(chan app.Update, 2)
	updates <- app.Update{Event: &app.Event{Type: "message.delta", Text: "PTY_RESPONSE_MARKER"}}
	updates <- app.Update{Result: &app.Result{ID: "pty-run", Text: "PTY_RESPONSE_MARKER", State: "completed"}}
	close(updates)
	return &app.Execution{ID: "pty-run", Updates: updates}
}
func (terminalExecutor) FollowUp(string, string) error { return nil }
func (terminalExecutor) Steer(string, string) error    { return nil }
